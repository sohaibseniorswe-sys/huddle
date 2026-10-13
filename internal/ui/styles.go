// Package ui holds the look of Huddle's screens: the colours and styles,
// and reusable view pieces (status bar, panes, text field, help line,
// menu). Every piece is a plain function that returns a string; none of
// them keeps state. The Bubble Tea models in package app decide what to
// show, and call these functions to draw it.
//
// The colours and styles are copied from docs/v3/screens/mockups.go, the
// visual reference for v3. Views never invent colours: they use these.
package ui

import "github.com/charmbracelet/lipgloss"

// The colour palette.
var (
	Purple = lipgloss.Color("#7D56F4") // focus, commands, Huddle's own badge
	Green  = lipgloss.Color("#3FB950") // done, CI green, the human ("you")
	Amber  = lipgloss.Color("#E3A008") // waiting, paused, the companion
	Blue   = lipgloss.Color("#58A6FF") // agent working, Codex
	Pink   = lipgloss.Color("#D2A8FF") // the PM, Claude Code
	Red    = lipgloss.Color("#F85149") // CI failed
	Text   = lipgloss.Color("#E6E6EF") // normal text
	Dim    = lipgloss.Color("#8B8B9A") // labels, hints, Huddle's events
	Faint  = lipgloss.Color("#4A4A5A") // unfocused borders, empty bar cells
	BarBg  = lipgloss.Color("#2A2A3A") // status bar background
	White  = lipgloss.Color("#FFFFFF")
	Black  = lipgloss.Color("#111111")
	// SelectBg is the background of a selected line.
	SelectBg = lipgloss.Color("#3A3456")
	// RawText is the colour of raw agent output in the log view.
	RawText = lipgloss.Color("#C9C9D6")
)

// Fg returns a style with foreground colour c.
func Fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// Common text styles.
var (
	Bold  = lipgloss.NewStyle().Bold(true).Foreground(Text)
	Plain = Fg(Text)
	Muted = Fg(Dim)
	Key   = lipgloss.NewStyle().Foreground(Text).Bold(true) // a key name in the help line
)
