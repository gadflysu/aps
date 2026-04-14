package preview

import "github.com/charmbracelet/lipgloss"

// ANSI 16-color palette — matches picker/styles.go for consistent theming.
var (
	// previewHeader styles section dividers, e.g. "━━━ SESSION INFO ━━━".
	previewHeader = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)

	// previewLabel* styles each field name.
	previewLabelTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // yellow
	previewLabelTime  = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // green
	previewLabelMsg   = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true) // magenta
	previewLabelDir   = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true) // dark grey

	// previewBullet styles the "•" preceding each recent message.
	previewBullet = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Bold(true)

	// previewMissing styles the "directory not found" error in listDir.
	previewMissing = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
