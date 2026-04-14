package picker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"local/aps/preview"
	"local/aps/source"
)

type state int

const (
	stateList        state = iota
	stateListPreview
)

// headerHeight is the number of terminal rows consumed by the search bar:
// one input line + two blank lines ("> query\n\n").
const headerHeight = 3

const minWidth, minHeight = 80, 10

// Model is the bubbletea model for the interactive session picker.
type Model struct {
	sessions []source.Session
	filtered []source.Session // subset after fuzzy filter; equals sessions when query=""
	cursor   int              // index into filtered
	query    string           // current search string
	state    state
	preview  viewport.Model
	search   textinput.Model
	width    int // terminal columns (from WindowSizeMsg)
	height   int // terminal rows   (from WindowSizeMsg)
	combined bool
	chosen   *source.Session // non-nil after Enter; signals tea.Quit
}

func newModel(sessions []source.Session, combined bool) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200

	return Model{
		sessions: sessions,
		filtered: sessions,
		search:   ti,
		preview:  viewport.New(0, 0),
		combined: combined,
	}
}

func (m Model) Init() tea.Cmd {
	return m.search.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Preview column allocation: m.width * 4/10 total.
		// previewBorder adds BorderLeft(1) + PaddingLeft(1) = 2 cols of chrome.
		// Viewport content width must be 2 less so the styled output fills
		// exactly m.width*4/10 columns.
		m.preview.Width = msg.Width*4/10 - 2
		// No top/bottom border in previewBorder, so no vertical chrome.
		m.preview.Height = msg.Height
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

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			if m.state == stateListPreview {
				m.loadPreview()
			}

		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			if m.state == stateListPreview {
				m.loadPreview()
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
		targets[i] = s.Title + " " + s.CWDDisplay
	}
	matches := fuzzy.Find(m.query, targets)
	m.filtered = make([]source.Session, len(matches))
	for i, match := range matches {
		m.filtered[i] = m.sessions[match.Index]
	}
}

// loadPreview calls the appropriate preview function synchronously and stores
// the result in m.preview. Safe to call when m.filtered is empty.
func (m *Model) loadPreview() {
	if len(m.filtered) == 0 {
		m.preview.SetContent("No sessions.")
		return
	}
	s := m.filtered[m.cursor]
	var buf strings.Builder
	if s.Client == source.ClientClaude {
		preview.RenderClaude(&buf, s.ID, s.ProjectPath, s.CWD)
	} else {
		preview.RenderOpencode(&buf, s.ID, s.CWD)
	}
	m.preview.SetContent(buf.String())
	m.preview.GotoTop()
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
	id := s.ID
	if s.Client == source.ClientClaude && len(id) > 12 {
		id = id[:12]
	}
	tSty, dSty, prefix := titleStyle, dirStyle, "  "
	if selected {
		tSty, dSty, prefix = titleStyleSel, dirStyleSel, "▶ "
	}

	sep := sepStyle.Render("｜")
	row := timeStyle.Render(s.Time.Format("2006-01-02 15:04:05")) + sep +
		tSty.Render(s.Title) + sep +
		idStyle.Render(id) + sep +
		msgStyle.Render(fmt.Sprintf("%d", s.MsgCount))
	if m.combined {
		row += sep + srcStyle.Render(s.Client.String())
	}
	row += sep + dSty.Render(s.CWDDisplay)
	return prefix + row
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
		right := previewBorder.Width(pw).Height(m.height).Render(m.preview.View())
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
