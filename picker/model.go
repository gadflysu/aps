// Package picker provides the bubbletea TUI for fuzzy-filtering and previewing sessions.
package picker

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/sahilm/fuzzy"

	"github.com/gadflysu/aps/dbg"
	"github.com/gadflysu/aps/display"
	"github.com/gadflysu/aps/preview"
	"github.com/gadflysu/aps/source"
	"github.com/gadflysu/aps/watcher"
)

// firstViewLogged is an atomic flag that ensures the "first View()" debug
// checkpoint fires exactly once per Run() invocation. Reset in Run() before
// tea.NewProgram so that back-to-back runs in tests are independent.
var firstViewLogged atomic.Bool

// spinnerFrames is the palindrome sequence for confirmed-active sessions,
// matching Claude Code's own spinner character set.
var spinnerFrames = []string{"·", "✢", "✳", "✶", "✻", "✽", "✽", "✻", "✶", "✳", "✢", "·"}


// activeConf represents the confidence level of an active session detection.
type activeConf uint8

const (
	activeGuessed   activeConf = 1 // proc CWD matches, not cache-confirmed
	activeConfirmed activeConf = 2 // confirmed via pid+lstart cache hit or proc match on live refresh
)

type tickMsg struct{}

const (
	tickInterval    = 120 * time.Millisecond
	procsPollInterval = 3 * time.Second
	// guessed spinner runs at 600 ms/frame = every 5 ticks (5 × 120 ms).
	slowTickDiv = 5
)

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// procsPollMsg carries a fresh proc snapshot collected by the poll goroutine.
// DetectActive is intentionally NOT called in the goroutine — it needs
// m.sessions which belongs to the main goroutine. The handler runs it instead.
type procsPollMsg struct {
	procs []source.ProcInfo
}

// scheduleProcsPollCmd schedules a background proc collection after procsPollInterval.
// The goroutine only calls CollectProcs (pure syscall, no shared state).
func scheduleProcsPollCmd() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(procsPollInterval)
		return procsPollMsg{procs: source.CollectProcs()}
	}
}

type state int

const (
	stateList        state = iota
	stateListPreview
)

type previewFocus int

const (
	focusMsgs previewFocus = iota
	focusDir
)

// headerHeight is the number of terminal rows consumed by the search bar and
// column header: one input line + two blank lines ("> query\n\n") + one header row.
const headerHeight = 4

const minWidth, minHeight = 80, 10

// sectionHeaderLines: one title text line (underlined) = 1 row.
// sectionSepLines: one separator line between sections = 1 row.
// infoContentLines: Agent / Title / Session ID / Time / Turns / Directory / Data = 7 rows.
// infoTotalHeight: total rows consumed by the SESSION INFO section.
const (
	sectionHeaderLines = 1
	sectionSepLines    = 1
	infoContentLines   = 7
	infoTotalHeight    = sectionHeaderLines + infoContentLines // 8
)

// RefreshMsg is sent by the watcher when one or more JSONL files have changed.
type RefreshMsg struct{ Paths []string }

// Model is the bubbletea model for the interactive session picker.
type Model struct {
	sessions     []source.Session
	filtered     []source.Session // subset after fuzzy filter; equals sessions when query=""
	cursor       int              // index into filtered
	query        string           // current search string
	state        state
	vpInfo       viewport.Model // SESSION INFO section
	vpMsgs       viewport.Model // RECENT MESSAGES section
	vpDir        viewport.Model // DIRECTORY section
	previewFocus previewFocus   // which section receives j/k scroll
	hasMsgs      bool           // whether current session has recent messages
	search       textinput.Model
	width        int // terminal columns (from WindowSizeMsg)
	height       int // terminal rows    (from WindowSizeMsg)
	combined     bool
	idColW       int // adaptive ID column width, computed once from all sessions
	msgColW      int // adaptive MSG column width, computed once from all sessions
	chosen       *source.Session // non-nil after Enter; signals tea.Quit

	w              *watcher.Watcher // nil when watcher is unavailable
	pendingRefresh []string         // paths buffered while in stateListPreview

	spinFrame  int                    // fast spinner frame index (confirmed)
	slowFrame  int // guessed spinner frame index (advances every 5 ticks)
	tickCount  int // total tick count, drives spinner frames
	activeConfs map[string]activeConf // sessions with a running process: guessed or confirmed

	pidCache *source.PIDCache  // persistent pid→sessionID mapping
	procs    []source.ProcInfo // running procs snapshot from startup (cwd→proc index)

	// matchIdx maps session ID to per-field matched rune offsets populated by applyFilter.
	// Inner keys are the fieldTS/fieldTitle/fieldID/fieldCWD constants. Nil when query is empty.
	matchIdx map[string]map[string][]int

	colOffset    int // horizontal scroll offset in display columns (0 = leftmost)
	maxColOffset int // cached max colOffset; updated by updateMaxColOffset()

	sgrBuf string // accumulates partial SGR mouse fragment split by ESC-disambiguation timer
}

func newModel(sessions []source.Session, combined bool, w *watcher.Watcher, cache *source.PIDCache) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.Focus()

	procs := source.CollectProcs()
	ar := source.DetectActive(sessions, procs, cache)
	activeConfs := make(map[string]activeConf, len(ar.Confirmed)+len(ar.Guessed))
	for id := range ar.Confirmed {
		activeConfs[id] = activeConfirmed
	}
	for id := range ar.Guessed {
		activeConfs[id] = activeGuessed
	}
	dbg.Log("[startup] procs=%d confirmed=%d guessed=%d", len(procs), len(ar.Confirmed), len(ar.Guessed))
	for id := range ar.Confirmed {
		dbg.Log("[startup] confirmed %s", id)
	}
	for id := range ar.Guessed {
		dbg.Log("[startup] guessed %s", id)
	}

	return Model{
		sessions:     sessions,
		filtered:     sessions,
		search:       ti,
		vpInfo:       viewport.New(0, 0),
		vpMsgs:       viewport.New(0, 0),
		vpDir:        viewport.New(0, 0),
		previewFocus: focusDir,
		combined:     combined,
		idColW:       adaptiveIDColW(sessions),
		msgColW:      display.AdaptiveMsgWidth(sessions),
		w:            w,
		activeConfs:  activeConfs,
		pidCache:     cache,
		procs:        procs,
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.search.Focus(), tickCmd(), scheduleProcsPollCmd()}
	if m.w != nil {
		cmds = append(cmds, waitForRefresh(m.w.C()))
	}
	return tea.Batch(cmds...)
}

// waitForRefresh blocks on the watcher channel and returns a RefreshMsg.
// Re-issued after every RefreshMsg to keep the loop alive.
func waitForRefresh(ch <-chan []string) tea.Cmd {
	return func() tea.Msg {
		return RefreshMsg{Paths: <-ch}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case RefreshMsg:
		if m.state == stateListPreview {
			hot, cold := m.splitPaths(msg.Paths)
			if len(hot) > 0 {
				m.applyRefresh(hot)
				m.loadPreview()
			}
			m.pendingRefresh = append(m.pendingRefresh, cold...)
		} else {
			m.applyRefresh(msg.Paths)
		}
		if m.w != nil {
			return m, waitForRefresh(m.w.C())
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.updatePreviewHeights()
		m.updateMaxColOffset()
		return m, nil

	case tea.MouseMsg:
		switch {
		case msg.Button == tea.MouseButtonWheelRight ||
			(msg.Shift && msg.Button == tea.MouseButtonWheelDown):
			if m.colOffset < m.maxColOffset {
				m.colOffset += hScrollStep
				if m.colOffset > m.maxColOffset {
					m.colOffset = m.maxColOffset
				}
			}
		case msg.Button == tea.MouseButtonWheelLeft ||
			(msg.Shift && msg.Button == tea.MouseButtonWheelUp):
			if m.colOffset > 0 {
				m.colOffset -= hScrollStep
				if m.colOffset < 0 {
					m.colOffset = 0
				}
			}
		case !msg.Shift && msg.Button == tea.MouseButtonWheelDown:
			if m.state == stateListPreview {
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineDown(1)
				case focusDir:
					m.vpDir.LineDown(1)
				}
			} else {
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
					m.updateMaxColOffset()
				}
			}
		case !msg.Shift && msg.Button == tea.MouseButtonWheelUp:
			if m.state == stateListPreview {
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineUp(1)
				case focusDir:
					m.vpDir.LineUp(1)
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					m.updateMaxColOffset()
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit

		case "esc":
			if m.state == stateListPreview {
				m.state = stateList
				return m, nil
			}
			if m.query != "" {
				m.search.SetValue("")
				m.query = ""
				m.applyFilter()
				return m, nil
			}
			return m, tea.Quit

		case "enter":
			if len(m.filtered) > 0 {
				s := m.filtered[m.cursor]
				m.chosen = &s
			}
			return m, tea.Quit

		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.updateMaxColOffset()
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.updateMaxColOffset()
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "k":
			if m.state == stateListPreview {
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineUp(1)
				case focusDir:
					m.vpDir.LineUp(1)
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
					m.updateMaxColOffset()
				}
			}

		case "j":
			if m.state == stateListPreview {
				switch m.previewFocus {
				case focusMsgs:
					m.vpMsgs.LineDown(1)
				case focusDir:
					m.vpDir.LineDown(1)
				}
			} else {
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
					m.updateMaxColOffset()
				}
			}

		case "left":
			if m.colOffset > 0 {
				m.colOffset -= hScrollStep
				if m.colOffset < 0 {
					m.colOffset = 0
				}
			}

		case "right":
			if m.colOffset < m.maxColOffset {
				m.colOffset += hScrollStep
				if m.colOffset > m.maxColOffset {
					m.colOffset = m.maxColOffset
				}
			}

		case "tab":
			if m.state == stateListPreview && m.hasMsgs {
				if m.previewFocus == focusMsgs {
					m.previewFocus = focusDir
				} else {
					m.previewFocus = focusMsgs
				}
			}

		case " ":
			if m.state == stateList {
				m.state = stateListPreview
				m.loadPreview()
			} else {
				m.state = stateList
				if len(m.pendingRefresh) > 0 {
					m.applyRefresh(m.pendingRefresh)
					m.pendingRefresh = nil
				}
			}

		default:
			// Strip SGR mouse fragments that leak when ESC is consumed by
			// bubbletea's disambiguation timer. Fragments may arrive as:
			//  - a single complete "[<digits;digits;digitsM"
			//  - multiple complete fragments in one message
			//  - a partial prefix split across consecutive messages (sgrBuf)
			combined := m.sgrBuf + string(msg.Runes)
			remainder, tail := consumeSGRFragments(combined)
			m.sgrBuf = tail // tail is a dangling prefix, keep for next message
			if remainder == "" {
				return m, nil
			}
			remainderMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(remainder)}
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(remainderMsg)
			newQuery := m.search.Value()
			if newQuery != m.query {
				m.query = newQuery
				m.applyFilter()
				m.cursor = 0
				m.colOffset = 0
				m.updateMaxColOffset()
				if m.state == stateListPreview {
					m.loadPreview()
				}
			}
			return m, cmd
		}

	case tickMsg:
		m.tickCount++
		m.spinFrame = m.tickCount % len(spinnerFrames)
		m.slowFrame = (m.tickCount / slowTickDiv) % len(spinnerFrames)
		return m, tickCmd()

	case procsPollMsg:
		if procsChanged(m.procs, msg.procs) {
			dbg.Log("[recheck] procs changed: %d → %d", len(m.procs), len(msg.procs))
		}
		m.procs = msg.procs
		ar := source.DetectActive(m.sessions, m.procs, m.pidCache)
		newConfs := make(map[string]activeConf, len(ar.Confirmed)+len(ar.Guessed))
		for id := range ar.Confirmed {
			newConfs[id] = activeConfirmed
		}
		for id := range ar.Guessed {
			newConfs[id] = activeGuessed
		}
		for id := range m.activeConfs {
			if newConfs[id] == 0 {
				dbg.Log("[recheck] deactivated %s", id)
			}
		}
		for id, conf := range newConfs {
			if m.activeConfs[id] == 0 {
				dbg.Log("[recheck] activated %s (conf=%d)", id, conf)
			}
		}
		m.activeConfs = newConfs
		return m, scheduleProcsPollCmd()
	}
	return m, nil
}

// procsChanged reports whether two proc slices differ by PID set.
func procsChanged(a, b []source.ProcInfo) bool {
	if len(a) != len(b) {
		return true
	}
	pids := make(map[string]bool, len(a))
	for _, p := range a {
		pids[p.PID+"|"+p.LStart] = true
	}
	for _, p := range b {
		if !pids[p.PID+"|"+p.LStart] {
			return true
		}
	}
	return false
}

// updatePreviewHeights recomputes all three viewport dimensions from m.width,
// m.height, and m.hasMsgs. Call after WindowSizeMsg or after loadPreview changes hasMsgs.
func (m *Model) updatePreviewHeights() {
	pw := m.width*4/10 - 2 // usable content width inside table left border
	m.vpInfo.Width = pw
	m.vpMsgs.Width = pw
	m.vpDir.Width = pw

	m.vpInfo.Height = infoContentLines

	available := m.height - infoTotalHeight

	if m.hasMsgs {
		available -= sectionSepLines + sectionHeaderLines // sep + msgs title row
		msgsH := available / 3
		if msgsH < 1 {
			msgsH = 1
		}
		m.vpMsgs.Height = msgsH
		available -= msgsH
	} else {
		m.vpMsgs.Height = 0
	}

	available -= sectionSepLines + sectionHeaderLines // sep + dir title row
	if available < 1 {
		available = 1
	}
	m.vpDir.Height = available
}

// applyFilter re-computes m.filtered from m.sessions using sahilm/fuzzy.
// Performance assumption: < 5 000 sessions → no debounce needed.
//
// Unicode/CJK note: sahilm/fuzzy stores byte offsets in MatchedIndexes, not
// rune indices. We build a byteToRune table per target to convert before
// comparing against the rune-unit fieldOffsets.
func (m *Model) applyFilter() {
	if m.query == "" {
		m.filtered = m.sessions
		m.matchIdx = nil
		return
	}
	targets := make([]string, len(m.sessions))
	offsets := make([]fieldOffsets, len(m.sessions))
	titleContentW := m.listTitleWidth() - 2 // content width = outer - PaddingLeft - PaddingRight
	for i, s := range m.sessions {
		ts := s.Time.Format("2006-01-02 15:04:05")
		title := display.TruncateWidth(display.Sanitize(s.Title), titleContentW, "…")
		id := display.TruncateWidth(s.ID, m.idColW, "")
		cwd := display.Sanitize(s.CWDDisplay)
		// Order matches display columns: TIME TITLE ID DIRECTORY
		// All fields match exactly what renderRowFull puts on screen.
		targets[i] = ts + " " + title + " " + id + " " + cwd
		sLen := len([]rune(ts))
		tLen := len([]rune(title))
		iLen := len([]rune(id))
		offsets[i] = fieldOffsets{
			tsEnd:      sLen,
			titleStart: sLen + 1,
			titleEnd:   sLen + 1 + tLen,
			idStart:    sLen + 1 + tLen + 1,
			idEnd:      sLen + 1 + tLen + 1 + iLen,
			cwdStart:   sLen + 1 + tLen + 1 + iLen + 1,
			cwdEnd:     sLen + 1 + tLen + 1 + iLen + 1 + len([]rune(cwd)),
		}
	}
	matches := fuzzy.Find(m.query, targets)
	m.filtered = make([]source.Session, len(matches))
	idx := make(map[string]map[string][]int, len(matches))
	for i, match := range matches {
		s := m.sessions[match.Index]
		m.filtered[i] = s
		off := offsets[match.Index]
		// Build byte→rune index table for this target.
		b2r := byteToRuneTable(targets[match.Index])
		fields := map[string][]int{fieldTS: nil, fieldTitle: nil, fieldID: nil, fieldCWD: nil}
		for _, byteIdx := range match.MatchedIndexes {
			ri, ok := b2r[byteIdx]
			if !ok {
				continue
			}
			switch {
			case ri < off.tsEnd:
				fields[fieldTS] = append(fields[fieldTS], ri)
			case ri >= off.titleStart && ri < off.titleEnd:
				fields[fieldTitle] = append(fields[fieldTitle], ri-off.titleStart)
			case ri >= off.idStart && ri < off.idEnd:
				fields[fieldID] = append(fields[fieldID], ri-off.idStart)
			case ri >= off.cwdStart && ri < off.cwdEnd:
				fields[fieldCWD] = append(fields[fieldCWD], ri-off.cwdStart)
			}
		}
		idx[s.ID] = fields
	}
	m.matchIdx = idx
	sort.Slice(m.filtered, func(i, j int) bool {
		return m.filtered[i].Time.After(m.filtered[j].Time)
	})
}

// byteToRuneTable converts sahilm/fuzzy's byte-offset MatchedIndexes into rune
// indices so they can be compared against the rune-unit fieldOffsets boundaries.
func byteToRuneTable(s string) map[int]int {
	m := make(map[int]int, len(s))
	ri := 0
	for bi := range s {
		m[bi] = ri
		ri++
	}
	return m
}

// matchField keys for matchIdx inner map.
const (
	fieldTS    = "ts"
	fieldTitle = "title"
	fieldID    = "id"
	fieldCWD   = "cwd"
)

type fieldOffsets struct {
	tsEnd, titleStart, titleEnd, idStart, idEnd, cwdStart, cwdEnd int
}

// highlightField renders s by applying hitSty to the contiguous runs of rune
// positions listed in runeIdxs, and baseSty to the rest.
// Returns a plain string (no outer Width/padding) suitable for embedding in a
// larger rendered cell.
func highlightField(s string, runeIdxs []int, baseSty, hitSty lipgloss.Style) string {
	if len(runeIdxs) == 0 {
		return baseSty.Render(s)
	}
	set := make(map[int]bool, len(runeIdxs))
	for _, i := range runeIdxs {
		set[i] = true
	}
	runes := []rune(s)
	var sb strings.Builder
	i := 0
	for i < len(runes) {
		if set[i] {
			start := i
			for i < len(runes) && set[i] {
				i++
			}
			sb.WriteString(hitSty.Render(string(runes[start:i])))
		} else {
			start := i
			for i < len(runes) && !set[i] {
				i++
			}
			sb.WriteString(baseSty.Render(string(runes[start:i])))
		}
	}
	return sb.String()
}

// loadPreview populates the three viewports for the currently selected session.
// Safe to call when m.filtered is empty.
func (m *Model) loadPreview() {
	if len(m.filtered) == 0 {
		m.vpInfo.SetContent("No sessions.")
		m.vpMsgs.SetContent("")
		m.vpDir.SetContent("")
		m.hasMsgs = false
		m.updatePreviewHeights()
		return
	}

	s := m.filtered[m.cursor]

	if s.Client == source.ClientClaude {
		m.vpInfo.SetContent(preview.ClaudeInfo(s.ID, s.ProjectPath, s.CWD))
		msgsContent := preview.ClaudeMsgs(s.ID, s.ProjectPath)
		m.hasMsgs = msgsContent != ""
		m.vpMsgs.SetContent(msgsContent)
	} else {
		m.vpInfo.SetContent(preview.OpencodeInfo(s.ID, s.CWD))
		m.hasMsgs = false
		m.vpMsgs.SetContent("")
	}

	m.vpDir.SetContent(preview.DirListing(s.CWD))

	if !m.hasMsgs {
		m.previewFocus = focusDir
	}

	m.updatePreviewHeights()
	m.updateMaxColOffset()
	m.vpInfo.GotoTop()
	m.vpMsgs.GotoTop()
	m.vpDir.GotoTop()
}

// updateMaxColOffset recomputes and caches the maximum horizontal scroll offset.
// Must be called whenever visible rows or terminal width may have changed.
func (m *Model) updateMaxColOffset() {
	scrollableW := m.scrollableWidth()
	vp := m.width - spinnerColW
	if vp < 0 {
		vp = 0
	}
	max := scrollableW - vp
	if max < 0 {
		max = 0
	}
	m.maxColOffset = max
	if m.colOffset > max {
		m.colOffset = max
	}
}

// scrollableWidth returns the maximum scrollable content width across all
// currently visible rows. Using the widest visible row ensures maxColOffset
// allows scrolling even when the cursor sits on a shorter row.
func (m Model) scrollableWidth() int {
	if len(m.filtered) == 0 {
		return 0
	}
	listHeight := m.height - headerHeight
	start, end := visibleRange(m.cursor, len(m.filtered), listHeight)
	max := 0
	for i := start; i < end; i++ {
		row := m.renderRowFull(m.filtered[i], i == m.cursor, false)
		scrollable := xansi.TruncateLeft(row, spinnerColW, "")
		if w := lipgloss.Width(scrollable); w > max {
			max = w
		}
	}
	return max
}

// cutScrollable applies m.colOffset to the scrollable portion of a rendered
// row, leaving the fixed spinnerColW prefix untouched.
// s is the full rendered row string (spinner prefix + scrollable content).
func (m Model) cutScrollable(s string) string {
	if m.colOffset <= 0 {
		return s
	}
	// Use display-column-aware split: keep first spinnerColW display columns
	// as the fixed spinner prefix, then skip spinnerColW+colOffset display
	// columns from the start to obtain the scrolled content.
	// Must NOT use s[:spinnerColW] (byte slice) — active spinner cells contain
	// multi-byte UTF-8 + ANSI sequences that are far longer than 2 bytes.
	spinnerPart := xansi.Truncate(s, spinnerColW, "")
	scrolledPart := xansi.TruncateLeft(s, spinnerColW+m.colOffset, "")
	return spinnerPart + scrolledPart
}

// visibleRange returns the [start, end) slice indices of sessions to render
// given cursor position, total count, and available row height.
// Extracted as a pure function for easier boundary-condition testing.
func visibleRange(cursor, total, height int) (start, end int) {
	start = 0
	if cursor >= height {
		start = cursor - height + 1
	}
	end = start + height
	if end > total {
		end = total
	}
	return
}

// renderColumnHeader renders a column-label row that aligns with renderRow output.
// Width values are outer (content + 2 padding) to match field styles.
func (m Model) renderColumnHeader() string {
	tw := m.listTitleWidth() // outer
	h := headerStyle.Copy().PaddingLeft(1).PaddingRight(1)
	// truncLabel truncates a header label to fit within an outer width (minus 2 padding).
	truncLabel := func(label string, outerW int) string {
		contentW := outerW - 2
		if contentW <= 0 {
			return label // let lipgloss handle overflow
		}
		return display.TruncateWidth(label, contentW, "")
	}
	row := "  " + // prefix width matches renderRow spinner cell (2 chars)
		h.Copy().Width(timeColW+2).Render(truncLabel("TIME", timeColW+2)) +
		h.Copy().Width(tw).Render(truncLabel("TITLE", tw))
	if m.state != stateListPreview {
		row += h.Copy().Width(m.idColW+2).Render(truncLabel("ID", m.idColW+2))
	}
	row += h.Copy().Width(m.msgColW+2).Render(truncLabel("TURNS", m.msgColW+2))
	if m.combined {
		row += h.Copy().Width(srcColW+2).Render(truncLabel("SRC", srcColW+2))
	}
	if m.state != stateListPreview {
		row += h.Copy().UnsetWidth().PaddingRight(0).Render("DIRECTORY")
	}
	return m.cutScrollable(row)
}

func (m Model) renderList() string {
	if len(m.filtered) == 0 {
		return dirStyle.Render("No matches.")
	}

	listHeight := m.height - headerHeight
	start, end := visibleRange(m.cursor, len(m.filtered), listHeight)

	var sb strings.Builder
	var prevDir string
	for i := start; i < end; i++ {
		s := m.filtered[i]
		dim := s.CWDDisplay == prevDir
		sb.WriteString(m.cutScrollable(m.renderRowFull(s, i == m.cursor, dim)))
		sb.WriteByte('\n')
		prevDir = s.CWDDisplay
	}
	return sb.String()
}

const maxIDColW = 12 // mirrors the original hard-coded idStyle Width(12)

// adaptiveIDColW returns min(AdaptiveIDWidth, maxIDColW) for the TUI.
// The cap prevents very long Opencode IDs from consuming too much horizontal space.
func adaptiveIDColW(sessions []source.Session) int {
	w := display.AdaptiveIDWidth(sessions)
	if w > maxIDColW {
		return maxIDColW
	}
	return w
}

// listTitleWidth returns the title column OUTER width (content + padding) for
// the current state. In lipgloss v2, Width() sets outer box width.
// In preview mode the list is narrowed to lw=width*6/10.
func (m Model) listTitleWidth() int {
	// All outer widths: spinner(2) + time(timeColW+2) + id(idColW+2) + msg(msgColW+2)
	fixed := 2 + (timeColW + 2) + (m.idColW + 2) + (m.msgColW + 2)
	if m.combined {
		fixed += srcColW + 2
	}

	if m.state == stateListPreview {
		lw := m.width * 6 / 10
		// Preview pane shows session ID; drop the ID column from the list.
		previewFixed := fixed - (m.idColW + 2)
		tw := lw - previewFixed
		if tw < 3 {
			tw = 3
		}
		return tw
	}

	// stateList: base width is titleColWidth; give surplus terminal width to title
	// up to the natural maximum title width (widest unsanitized title).
	tw := titleColWidth + 2 // outer = content + PaddingLeft + PaddingRight
	if m.width > 0 {
		maxNaturalTitle := 0
		for _, s := range m.sessions {
			if w := lipgloss.Width(display.Sanitize(s.Title)); w > maxNaturalTitle {
				maxNaturalTitle = w
			}
		}
		maxBonus := maxNaturalTitle - titleColWidth
		if maxBonus > 0 {
			// dir column is unconstrained in stateList; estimate its natural width.
			maxDirW := 0
			for _, s := range m.sessions {
				if w := lipgloss.Width(display.Sanitize(s.CWDDisplay)); w > maxDirW {
					maxDirW = w
				}
			}
			// +2: PaddingLeft+PaddingRight for dir cell border spaces
			naturalW := fixed + tw + maxDirW + 2
			if surplus := m.width - naturalW; surplus > 0 {
				if surplus > maxBonus {
					surplus = maxBonus
				}
				tw += surplus
			}
		}
	}
	return tw
}

func (m Model) renderRowDim(s source.Session, selected bool) string {
	return m.renderRowFull(s, selected, true)
}

func (m Model) renderRow(s source.Session, selected bool) string {
	return m.renderRowFull(s, selected, false)
}

// cellHLStyles returns the base and highlight styles for a cell,
// applying reverse video when the row is selected.
func cellHLStyles(fg color.Color, selected bool) (base lipgloss.Style, hit lipgloss.Style) {
	base = lipgloss.NewStyle().Foreground(fg)
	hit = matchStyle
	if selected {
		base = base.Reverse(true)
		hit = hit.Reverse(true)
	}
	return base, hit
}

func (m Model) renderRowFull(s source.Session, selected bool, dimDir bool) string {
	previewMode := m.state == stateListPreview

	timeSty, tSty, msgSty, srcSty :=
		timeStyle, titleStyle, msgStyle, srcStyle
	if selected {
		timeSty, tSty, msgSty, srcSty =
			timeStyleSel, titleStyleSel, msgStyleSel, srcStyleSel
	}

	// Spinner cell: confirmed → fast spinner; guessed → slow spinner; inactive → spaces.
	spinCell := "  "
	switch m.activeConfs[s.ID] {
	case activeConfirmed:
		spinCell = spinnerStyleConfirmed.Render(spinnerFrames[m.spinFrame%len(spinnerFrames)]) + " "
	case activeGuessed:
		spinCell = spinnerStyleGuessed.Render(spinnerFrames[m.slowFrame%len(spinnerFrames)]) + " "
	}

	tw := m.listTitleWidth() // outer width (content + 2 padding)
	titleContent := display.TruncateWidth(display.Sanitize(s.Title), tw-2, "…")
	tsContent := s.Time.Format("2006-01-02 15:04:05")
	cwdContent := display.Sanitize(s.CWDDisplay)
	hi := m.matchIdx[s.ID]
	var renderedTime, renderedTitle string
	if hi != nil {
		// Segment-level rendering keeps base colour after matchStyle reset.
		tsBase, hitSty := cellHLStyles(timeSty.GetForeground(), selected)
		tBase, _ := cellHLStyles(tSty.GetForeground(), selected)
		renderedTime = timeSty.Render(highlightField(tsContent, hi[fieldTS], tsBase, hitSty))
		titleBody := highlightField(titleContent, hi[fieldTitle], tBase, hitSty)
		renderedTitle = tSty.Copy().Width(tw).Render(titleBody)
	} else {
		renderedTime = timeSty.Render(tsContent)
		renderedTitle = tSty.Copy().Width(tw).Render(titleContent)
	}
	row := renderedTime +
		renderedTitle
	// In preview mode the ID is shown in the preview pane; omit the column here.
	if !previewMode {
		idSty := idStyle
		if selected {
			idSty = idStyleSel
		}
		id := display.TruncateWidth(s.ID, m.idColW, "")
		var renderedID string
		if hi != nil {
			idBase, idHit := cellHLStyles(idSty.GetForeground(), selected)
			idBody := highlightField(id, hi[fieldID], idBase, idHit)
			renderedID = idSty.Copy().Width(m.idColW+2).Render(idBody)
		} else {
			renderedID = idSty.Copy().Width(m.idColW+2).Render(id)
		}
		row += renderedID
	}
	row += msgSty.Copy().Width(m.msgColW+2).Render(fmt.Sprintf("%d", s.MsgCount))
	if m.combined {
		row += srcSty.Render(s.Client.String())
	}
	// In preview mode the dir is already shown in the preview pane; omit it here.
	if !previewMode {
		if hi != nil {
			// Highlight matches in cwd; selected rows get red+reverse.
			cwdBase, cwdHit := cellHLStyles(display.ColorDir, selected)
			row += dirStyle.Render(" ") + highlightField(cwdContent, hi[fieldCWD], cwdBase, cwdHit) + dirStyle.Render(" ")
		} else if selected {
			row += dirStyleSel.Render(cwdContent)
		} else {
			row += dirStyle.Render(" ") + display.FormatDirCell(cwdContent, 0, dimDir) + dirStyle.Render(" ")
		}
	}
	return spinCell + row
}

// renderPreviewPane composes the three section panels as a single-column table.
// BorderLeft+BorderRow produces automatic ├── junctions; top/bottom/right are off.
func (m Model) renderPreviewPane() string {
	pw := m.width*4/10 - 2

	borderSty := lipgloss.NewStyle().Foreground(display.ColorMuted)

	infoTitle := lipgloss.NewStyle().Bold(true).Underline(true).
		Foreground(display.ColorHeader).Width(pw).Render("SESSION INFO")
	infoCell := lipgloss.JoinVertical(lipgloss.Top, infoTitle, m.vpInfo.View())

	rows := []string{infoCell}

	if m.hasMsgs {
		var msgsColor color.Color = display.ColorHeader
		if m.previewFocus == focusMsgs {
			msgsColor = display.ColorMsg
		}
		msgsTitle := lipgloss.NewStyle().Bold(true).Underline(true).
			Foreground(msgsColor).Width(pw).Render("RECENT MESSAGES")
		msgsCell := lipgloss.JoinVertical(lipgloss.Top, msgsTitle, m.vpMsgs.View())
		rows = append(rows, msgsCell)
	}

	var dirColor color.Color = display.ColorHeader
	if m.previewFocus == focusDir {
		dirColor = display.ColorDir
	}
	dirTitle := lipgloss.NewStyle().Bold(true).Underline(true).
		Foreground(dirColor).Width(pw).Render("DIRECTORY")
	dirCell := lipgloss.JoinVertical(lipgloss.Top, dirTitle, m.vpDir.View())
	rows = append(rows, dirCell)

	cell := lipgloss.NewStyle().Width(pw)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderTop(false).
		BorderBottom(false).
		BorderRight(false).
		BorderRow(true).
		BorderColumn(false).
		BorderStyle(borderSty).
		StyleFunc(func(row, col int) lipgloss.Style { return cell })
	for _, r := range rows {
		t.Row(r)
	}
	return t.Render()
}

func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (need %dx%d, got %dx%d)",
			minWidth, minHeight, m.width, m.height)
	}

	if firstViewLogged.CompareAndSwap(false, true) {
		dbg.Log("first View()")
	}

	searchBar := "> " + m.search.View() + "\n\n" // 3 rows
	colHeader := m.renderColumnHeader() + "\n"   // 1 row; total = headerHeight(4)
	list := m.renderList()

	if m.state == stateListPreview {
		lw := m.width * 6 / 10
		left := lipgloss.NewStyle().Width(lw).Render(searchBar + colHeader + list)
		right := m.renderPreviewPane()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return searchBar + colHeader + list
}

// Run starts the interactive session picker and returns the chosen session,
// or nil if the user cancelled. combined=true shows the SRC column.
func Run(sessions []source.Session, combined bool, cache *source.PIDCache) (*source.Session, error) {
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, ".claude", "projects")

	w, _ := watcher.New(baseDir) // failure degrades to poll-only; w is never nil

	m := newModel(sessions, combined, w, cache)
	firstViewLogged.Store(false)
	dbg.Log("picker.Run start")
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	final, err := p.Run()
	w.Stop()
	if err != nil {
		return nil, err
	}
	result, ok := final.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	return result.chosen, nil
}

// applyRefresh reloads the given JSONL paths, updates m.sessions, re-sorts,
// and anchors the cursor to the previously selected session by ID.
// Sessions that are successfully updated are also marked active: a file change
// received while aps is running means the session is live.
func (m *Model) applyRefresh(paths []string) {
	if len(paths) == 0 {
		return
	}

	// Build ID→index map for existing sessions.
	byID := make(map[string]int, len(m.sessions))
	for i, s := range m.sessions {
		byID[s.ID] = i
	}

	// Remember cursor session ID for re-anchoring.
	var cursorID string
	if len(m.filtered) > 0 && m.cursor < len(m.filtered) {
		cursorID = m.filtered[m.cursor].ID
	}

	for _, path := range paths {
		updated, err := source.ReloadSession(path, false)
		if err != nil {
			continue
		}
		if idx, exists := byID[updated.ID]; exists {
			if prev := m.sessions[idx]; prev.Title != updated.Title {
				dbg.Log("[applyRefresh] %s title changed: %q → %q", updated.ID, prev.Title, updated.Title)
			}
			m.sessions[idx] = updated
		} else {
			dbg.Log("[applyRefresh] new session %s (title=%q)", updated.ID, updated.Title)
			m.sessions = append(m.sessions, updated)
			byID[updated.ID] = len(m.sessions) - 1
		}
		// Mark active only if a live process matches this session's CWD.
		// A file change without a matching process means the session just exited.
		if m.activeConfs == nil {
			m.activeConfs = make(map[string]activeConf)
		}
		if !cwdInProcs(m.procs, updated.CWD) {
			// CWD not in snapshot at all — proc may have started after aps launched.
			m.procs = source.CollectProcs()
			dbg.Log("[applyRefresh] proc snapshot refreshed (%d procs)", len(m.procs))
		}
		match := findUniqueProc(m.procs, updated.CWD)
		if match != nil {
			if m.activeConfs[updated.ID] == 0 {
				dbg.Log("[applyRefresh] marking active via live refresh: %s (path=%s)", updated.ID, path)
			}
			m.activeConfs[updated.ID] = activeConfirmed
			if m.pidCache != nil && m.pidCache.Lookup(*match) == "" {
				dbg.Log("[applyRefresh] cache set pid=%s lstart=%q → %s", match.PID, match.LStart, updated.ID)
				m.pidCache.Set(*match, updated.ID)
			}
			m.evictGuessedSiblings(updated.ID, updated.CWD, *match)
		}
	}

	sort.Slice(m.sessions, func(i, j int) bool {
		return m.sessions[i].Time.After(m.sessions[j].Time)
	})

	m.reguessActive()

	m.applyFilter()
	m.updateMaxColOffset()

	// Re-anchor cursor by ID.
	if cursorID != "" {
		for i, s := range m.filtered {
			if s.ID == cursorID {
				m.cursor = i
				return
			}
		}
	}
	m.cursor = 0
}

// reguessActive re-evaluates the Guessed set after sessions have been updated.
// It runs DetectActive with the current proc snapshot, then replaces all
// activeGuessed entries with the fresh result while leaving Confirmed entries
// untouched.
func (m *Model) reguessActive() {
	ar := source.DetectActive(m.sessions, m.procs, m.pidCache)
	for id, conf := range m.activeConfs {
		if conf == activeGuessed {
			delete(m.activeConfs, id)
		}
	}
	// Do not downgrade a Confirmed entry to Guessed. applyRefresh may have
	// confirmed a session via findUniqueProc without writing to pidCache (e.g.
	// when pidCache is nil), so DetectActive — which relies on the cache —
	// may put the same session in ar.Guessed instead of ar.Confirmed.
	for id := range ar.Guessed {
		if m.activeConfs[id] != activeConfirmed {
			m.activeConfs[id] = activeGuessed
		}
	}
}

// evictGuessedSiblings removes guessed sessions that share cwd from activeConfs,
// but only if every proc for that cwd is now accounted for in the cache.
//
// Precondition: confirmedProc has just been mapped to confirmedID.
// If any other proc for the same cwd has no cache entry, that proc may still
// belong to a sibling session, so siblings are left untouched.
func (m *Model) evictGuessedSiblings(confirmedID, cwd string, confirmedProc source.ProcInfo) {
	// Check whether all procs for this CWD are now confirmed (cache-mapped).
	for _, p := range m.procs {
		if p.CWD != cwd {
			continue
		}
		if p.PID == confirmedProc.PID && p.LStart == confirmedProc.LStart {
			continue // this is the proc we just confirmed
		}
		// Another proc exists for the same CWD with no cache entry — may belong to a sibling.
		if m.pidCache == nil || m.pidCache.Lookup(p) == "" {
			return
		}
	}
	// All procs for cwd are accounted for — evict guessed siblings.
	for _, s := range m.sessions {
		if s.ID != confirmedID && s.CWD == cwd && m.activeConfs[s.ID] == activeGuessed {
			dbg.Log("[applyRefresh] evicting guessed sibling %s (all procs for CWD confirmed to %s)", s.ID, confirmedID)
			delete(m.activeConfs, s.ID)
		}
	}
}

// splitPaths partitions paths into hot (current cursor session's JSONL) and cold
// (everything else). Cold paths are buffered; hot paths trigger an immediate
// preview reload. Returns all-cold when filtered is empty or cursor is out of range.
func (m *Model) splitPaths(paths []string) (hot, cold []string) {
	if len(m.filtered) == 0 || m.cursor >= len(m.filtered) {
		return nil, paths
	}
	cur := m.filtered[m.cursor]
	curPath := filepath.Join(cur.ProjectPath, cur.ID+".jsonl")
	for _, p := range paths {
		if p == curPath {
			hot = append(hot, p)
		} else {
			cold = append(cold, p)
		}
	}
	return hot, cold
}

var (
	// reSGRFull matches one complete SGR mouse fragment: "[<digits;...M/m"
	reSGRFull = regexp.MustCompile(`\[<[\d;]+[Mm]`)
	// reSGRTail matches an incomplete SGR prefix at the end of the string.
	reSGRTail = regexp.MustCompile(`\[<?[\d;]*$`)
)

// consumeSGRFragments strips SGR mouse fragments that leak into KeyRunes when
// ESC is consumed by bubbletea's disambiguation timer.
// Returns (remainder, tail): remainder is forwarded to the search box;
// tail is a dangling incomplete prefix to buffer until the next message.
func consumeSGRFragments(s string) (remainder, tail string) {
	s = reSGRFull.ReplaceAllString(s, "")
	if loc := reSGRTail.FindStringIndex(s); loc != nil {
		return s[:loc[0]], s[loc[0]:]
	}
	return s, ""
}

// findUniqueProc returns the single ProcInfo whose CWD matches, or nil if there
// are zero or multiple matches (ambiguous → can't determine which proc owns the session).
func findUniqueProc(procs []source.ProcInfo, cwd string) *source.ProcInfo {
	var match *source.ProcInfo
	for i := range procs {
		if procs[i].CWD == cwd {
			if match != nil {
				return nil // ambiguous
			}
			match = &procs[i]
		}
	}
	return match
}

// cwdInProcs reports whether any proc in the snapshot has the given CWD.
// Used instead of findUniqueProc==nil to avoid CollectProcs when CWD is present
// but ambiguous (>1 match) — refreshing would discard known procs and break reguessActive.
func cwdInProcs(procs []source.ProcInfo, cwd string) bool {
	for _, p := range procs {
		if p.CWD == cwd {
			return true
		}
	}
	return false
}
