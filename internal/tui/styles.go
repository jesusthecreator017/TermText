package tui

import "github.com/charmbracelet/lipgloss"

var (
	colAccent = lipgloss.Color("63")  // brand purple
	colSelf   = lipgloss.Color("42")  // own / online green
	colDim    = lipgloss.Color("241") // muted
	colText   = lipgloss.Color("250")
	colError  = lipgloss.Color("196")
	colWarn   = lipgloss.Color("214")
	colBorder = lipgloss.Color("237")
)

// styles is built once and reused across renders to avoid per-frame allocation.
type styles struct {
	title      lipgloss.Style
	header     lipgloss.Style
	dim        lipgloss.Style
	errorText  lipgloss.Style
	warn       lipgloss.Style
	timestamp  lipgloss.Style
	selfName   lipgloss.Style
	otherName  lipgloss.Style
	roomItem   lipgloss.Style
	roomActive lipgloss.Style
	roomsPane  lipgloss.Style
	selected   lipgloss.Style
	lock       lipgloss.Style
}

func newStyles() styles {
	return styles{
		title:     lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		header:    lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		dim:       lipgloss.NewStyle().Foreground(colDim),
		errorText: lipgloss.NewStyle().Foreground(colError),
		warn:      lipgloss.NewStyle().Foreground(colWarn),
		timestamp: lipgloss.NewStyle().Foreground(colDim),
		selfName:  lipgloss.NewStyle().Foreground(colSelf).Bold(true),
		otherName: lipgloss.NewStyle().Foreground(colAccent).Bold(true),
		roomItem:  lipgloss.NewStyle().Foreground(colText),
		roomActive: lipgloss.NewStyle().Foreground(colSelf).Bold(true),
		roomsPane: lipgloss.NewStyle().
			Width(leftPaneWidth).
			BorderStyle(lipgloss.NormalBorder()).
			BorderRight(true).
			BorderForeground(colBorder),
		selected: lipgloss.NewStyle().Foreground(colSelf).Bold(true),
		lock:     lipgloss.NewStyle().Foreground(colDim),
	}
}
