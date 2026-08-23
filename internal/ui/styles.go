package ui

import "github.com/charmbracelet/lipgloss"

var (
	cAccent = lipgloss.AdaptiveColor{Light: "#B58900", Dark: "#E3B341"} // gold
	cGreen  = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	cRed    = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	cDim    = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#8B949E"}
	cFaint  = lipgloss.AdaptiveColor{Light: "#AFB8C1", Dark: "#484F58"}

	sBrand   = lipgloss.NewStyle().Foreground(cAccent).Bold(true)
	sDim     = lipgloss.NewStyle().Foreground(cDim)
	sFaint   = lipgloss.NewStyle().Foreground(cFaint)
	sTotal   = lipgloss.NewStyle().Bold(true)
	sUp      = lipgloss.NewStyle().Foreground(cGreen)
	sDown    = lipgloss.NewStyle().Foreground(cRed)
	sUpB     = lipgloss.NewStyle().Foreground(cGreen).Bold(true)
	sDownB   = lipgloss.NewStyle().Foreground(cRed).Bold(true)
	sHeader  = lipgloss.NewStyle().Foreground(cDim).Bold(true)
	sErr     = lipgloss.NewStyle().Foreground(cRed)
	sSpinner = lipgloss.NewStyle().Foreground(cAccent)
	sKey     = lipgloss.NewStyle().Foreground(cAccent)
)

func paint(v float64) lipgloss.Style {
	if v < 0 {
		return sDown
	}
	return sUp
}
