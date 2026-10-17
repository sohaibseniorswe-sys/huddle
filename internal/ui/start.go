package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The start menu's pieces (screens 14 and 15 in docs/v3/screens).

// StartOptionWidth is the column width of an option's name in the start
// menu; its note starts after it.
const StartOptionWidth = 42

// StartBar draws the start menu's first row: the HUDDLE badge and where
// Huddle runs ("example · ~/projects/example").
func StartBar(width int, where string) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(White).Background(Purple).Padding(0, 1).Render("HUDDLE")
	room := width - lipgloss.Width(head)
	where = ansi.Truncate(where, max(room-2, 0), "…")
	rest := lipgloss.NewStyle().Foreground(Text).Background(BarBg).Padding(0, 1).
		Width(max(room, 0)).MaxWidth(max(room, 0)).Render(where)
	return head + rest
}

// StartOption draws one option of the start menu: "> " and bold when
// selected, then the note in grey.
func StartOption(name, note string, selected bool) string {
	if selected {
		return Fg(Purple).Bold(true).Render("> ") + Bold.Width(StartOptionWidth).Render(name) + Muted.Render(note)
	}
	return "  " + Plain.Width(StartOptionWidth).Render(name) + Muted.Render(note)
}

// StartCard draws the start menu's card: a purple box of the given width
// with a bold title, the body and a grey footer.
func StartCard(width int, title, body, foot string) string {
	parts := []string{Bold.Render(title), body}
	if foot != "" {
		parts = append(parts, Muted.Render(foot))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Purple).
		Padding(1, 2).Width(width).Render(strings.Join(parts, "\n\n"))
}
