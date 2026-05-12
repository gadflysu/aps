package picker

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	lipgloss "charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/sahilm/fuzzy"

	"github.com/gadflysu/aps/display"
	"github.com/gadflysu/aps/preview"
	"github.com/gadflysu/aps/source"
)

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
// infoContentLines: Title / Time / Messages / Directory = 4 rows.
// infoTotalHeight: total rows consumed by the SESSION INFO section.
const (
	sectionHeaderLines = 1
	sectionSepLines    = 1
	infoContentLines   = 4
	infoTotalHeight    = sectionHeaderLines + infoContentLines // 5
)

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
}

func newModel(sessions []source.Session, combined bool) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.Focus()

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
	}
}

func (m Model) Init() tea.Cmd {
	return m.search.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.updatePreviewHeights()
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
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
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
			}

		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			newQuery := m.search.Value()
			if newQuery != m.query {
				m.query = newQuery
				m.applyFilter()
				m.cursor = 0
				if m.state == stateListPreview {
					m.loadPreview()
				}
			}
			return m, cmd
		}
	}
	return m, nil
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
// Unicode/CJK: sahilm/fuzzy iterates rune indices (not bytes), so CJK
// characters in titles and paths are matched correctly as individual runes.
func (m *Model) applyFilter() {
	if m.query == "" {
		m.filtered = m.sessions
		return
	}
	targets := make([]string, len(m.sessions))
	for i, s := range m.sessions {
		targets[i] = s.Title + " " + s.CWDDisplay + " " + s.ID + " " + s.Time.Format("2006-01-02 15:04:05")
	}
	matches := fuzzy.Find(m.query, targets)
	m.filtered = make([]source.Session, len(matches))
	for i, match := range matches {
		m.filtered[i] = m.sessions[match.Index]
	}
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
	m.vpInfo.GotoTop()
	m.vpMsgs.GotoTop()
	m.vpDir.GotoTop()
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
	row := " " + // prefix width matches renderRow " " / "▶"
		h.Copy().Width(19+2).Render("TIME") +
		h.Copy().Width(tw).Render("TITLE") +
		h.Copy().Width(m.idColW+2).Render("ID") +
		h.Copy().Width(m.msgColW+2).Render("TURNS")
	if m.combined {
		row += h.Copy().Width(11+2).Render("SRC")
	}
	if m.state != stateListPreview {
		row += h.Copy().UnsetWidth().PaddingRight(0).Render("DIRECTORY")
	}
	return row
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
		sb.WriteString(m.renderRowFull(s, i == m.cursor, dim))
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
	if m.state != stateListPreview {
		return titleColWidth + 2 // outer = content + PaddingLeft + PaddingRight
	}
	lw := m.width * 6 / 10
	// All outer widths: prefix(1) + time(21) + id(idColW+2) + msg(msgColW+2)
	fixed := 1 + (19 + 2) + (m.idColW + 2) + (m.msgColW + 2)
	if m.combined {
		fixed += 11 + 2
	}
	tw := lw - fixed
	if tw < 3 { // minimum: 1 content col + 2 padding
		tw = 3
	}
	return tw
}

func (m Model) renderRowDim(s source.Session, selected bool) string {
	return m.renderRowFull(s, selected, true)
}

func (m Model) renderRow(s source.Session, selected bool) string {
	return m.renderRowFull(s, selected, false)
}

func (m Model) renderRowFull(s source.Session, selected bool, dimDir bool) string {
	id := display.TruncateWidth(s.ID, m.idColW, "")

	timeSty, tSty, idSty, msgSty, srcSty, prefix :=
		timeStyle, titleStyle, idStyle, msgStyle, srcStyle, " "
	if selected {
		timeSty, tSty, idSty, msgSty, srcSty, prefix =
			timeStyleSel, titleStyleSel, idStyleSel, msgStyleSel, srcStyleSel, "▶"
	}

	tw := m.listTitleWidth() // outer width (content + 2 padding)
	row := timeSty.Render(s.Time.Format("2006-01-02 15:04:05")) +
		tSty.Copy().Width(tw).Render(display.TruncateWidth(display.Sanitize(s.Title), tw-2, "…")) +
		idSty.Copy().Width(m.idColW+2).Render(id) +
		msgSty.Copy().Width(m.msgColW+2).Render(fmt.Sprintf("%d", s.MsgCount))
	if m.combined {
		row += srcSty.Render(s.Client.String())
	}
	// In preview mode the dir is already shown in the preview pane; omit it here.
	if m.state != stateListPreview {
		if selected {
			row += dirStyleSel.Render(s.CWDDisplay)
		} else {
			row += dirStyle.Render(" ") + display.FormatDirCell(display.Sanitize(s.CWDDisplay), 0, dimDir) + dirStyle.Render(" ")
		}
	}
	return prefix + row
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
func Run(sessions []source.Session, combined bool) (*source.Session, error) {
	m := newModel(sessions, combined)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	result, ok := final.(Model)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}
	return result.chosen, nil
}
