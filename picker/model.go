package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// headerHeight is the number of terminal rows consumed by the search bar:
// one input line + two blank lines ("> query\n\n").
const headerHeight = 3

const minWidth, minHeight = 80, 10

// sectionHeaderLines: one title text line + one bottom-border line = 2 rows.
// infoContentLines: Title / Time / Messages / Directory = 4 rows.
// infoTotalHeight: total rows consumed by the SESSION INFO section.
const (
	sectionHeaderLines = 2
	infoContentLines   = 4
	infoTotalHeight    = sectionHeaderLines + infoContentLines // 6
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
	height       int // terminal rows   (from WindowSizeMsg)
	combined     bool
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
		case "ctrl+c", "esc", "q":
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
	pw := m.width*4/10 - 2 // usable content width inside previewBorder chrome
	m.vpInfo.Width = pw
	m.vpMsgs.Width = pw
	m.vpDir.Width = pw

	m.vpInfo.Height = infoContentLines

	available := m.height - infoTotalHeight

	if m.hasMsgs {
		available -= sectionHeaderLines // account for msgs title row
		msgsH := available / 3
		if msgsH < 1 {
			msgsH = 1
		}
		m.vpMsgs.Height = msgsH
		available -= msgsH
	} else {
		m.vpMsgs.Height = 0
	}

	available -= sectionHeaderLines // account for dir title row
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

func (m Model) renderList() string {
	if len(m.filtered) == 0 {
		return dirStyle.Render("No matches.")
	}

	listHeight := m.height - headerHeight
	start, end := visibleRange(m.cursor, len(m.filtered), listHeight)

	var sb strings.Builder
	for i := start; i < end; i++ {
		sb.WriteString(m.renderRow(m.filtered[i], i == m.cursor))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func (m Model) renderRow(s source.Session, selected bool) string {
	id := display.TruncateWidth(s.ID, 12, "")

	timeSty, tSty, idSty, msgSty, srcSty, dSty, sepSty, prefix :=
		timeStyle, titleStyle, idStyle, msgStyle, srcStyle, dirStyle, sepStyle, "  "
	if selected {
		timeSty, tSty, idSty, msgSty, srcSty, dSty, sepSty, prefix =
			timeStyleSel, titleStyleSel, idStyleSel, msgStyleSel, srcStyleSel, dirStyleSel, sepStyleSel, "▶ "
	}

	sep := sepSty.Render("｜")
	row := timeSty.Render(s.Time.Format("2006-01-02 15:04:05")) + sep +
		tSty.Render(display.TruncateWidth(display.Sanitize(s.Title), titleColWidth, "…")) + sep +
		idSty.Render(id) + sep +
		msgSty.Render(fmt.Sprintf("%d", s.MsgCount))
	if m.combined {
		row += sep + srcSty.Render(s.Client.String())
	}
	row += sep + dSty.Render(s.CWDDisplay)
	return prefix + row
}

// renderSectionPanel renders a section as: title line (with bottom border) + viewport content.
// focused=true uses cyan (display.ColorDir) for the title/border to indicate scroll focus.
func renderSectionPanel(title, content string, width int, focused bool) string {
	fg := display.ColorMuted
	if focused {
		fg = display.ColorDir
	}
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottomForeground(fg).
		Width(width).
		Render(title)
	return lipgloss.JoinVertical(lipgloss.Top, header, content)
}

// renderPreviewPane composes the three section panels vertically.
func (m Model) renderPreviewPane() string {
	pw := m.width*4/10 - 2

	sections := []string{
		renderSectionPanel("SESSION INFO", m.vpInfo.View(), pw, false),
	}

	if m.hasMsgs {
		sections = append(sections,
			renderSectionPanel("RECENT MESSAGES", m.vpMsgs.View(), pw, m.previewFocus == focusMsgs),
		)
	}

	sections = append(sections,
		renderSectionPanel("DIRECTORY", m.vpDir.View(), pw, m.previewFocus == focusDir),
	)

	return lipgloss.JoinVertical(lipgloss.Top, sections...)
}

func (m Model) View() string {
	if m.width < minWidth || m.height < minHeight {
		return fmt.Sprintf("Terminal too small (need %dx%d, got %dx%d)",
			minWidth, minHeight, m.width, m.height)
	}

	header := "> " + m.search.View() + "\n\n" // headerHeight rows
	list := m.renderList()

	if m.state == stateListPreview {
		lw := m.width * 6 / 10
		pw := m.width - lw
		left := lipgloss.NewStyle().Width(lw).Render(header + list)
		right := previewBorder.Width(pw).Height(m.height).Render(m.renderPreviewPane())
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return header + list
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
