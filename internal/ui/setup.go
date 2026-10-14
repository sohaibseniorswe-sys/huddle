package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The setup wizard's pieces (screens 11, 12 and 13 in docs/v3/screens).

// FieldLabelWidth is the column width of a field's label in a wizard
// card ("CLI", "Model", "Effort"); the field's value starts after it.
const FieldLabelWidth = 9

// FieldMarkWidth is the width of the "▸ " mark in front of a focused field.
const FieldMarkWidth = 2

// SetupBar draws the wizard's first row: the HUDDLE SETUP badge, where
// Huddle is being set up (a folder), and "step N of total" on the right.
func SetupBar(width int, where string, step, total int) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(White).Background(Purple).Padding(0, 1).Render("HUDDLE SETUP")
	right := lipgloss.NewStyle().Foreground(Text).Background(BarBg).Padding(0, 1).Render(fmt.Sprintf("step %d of %d", step, total))
	room := width - lipgloss.Width(head) - lipgloss.Width(right)
	middle := lipgloss.NewStyle().Foreground(Text).Background(BarBg).Padding(0, 1).
		Width(max(room, 0)).MaxWidth(max(room, 0)).Render(where)
	return head + middle + right
}

// StepRow draws one row of the wizard's step list: a done step with ✓ and
// its value, the current step with ▸, or a later step dimmed.
func StepRow(n int, name, value string, current int) string {
	label := fmt.Sprintf("%d  %-18s", n, name)
	switch {
	case n < current:
		return Fg(Green).Render("  ✓ ") + Plain.Render(label) + Muted.Render(value)
	case n == current:
		return Fg(Purple).Bold(true).Render("  ▸ ") + Bold.Render(label)
	default:
		return Muted.Render("    " + label)
	}
}

// Chips draws a row of choices with the one at selected highlighted:
// purple while its field has focus (active), grey otherwise.
func Chips(options []string, selected int, active bool) string {
	var out []string
	for i, o := range options {
		switch {
		case i == selected && active:
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(White).Background(Purple).Padding(0, 1).Render(o))
		case i == selected:
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(Text).Background(BarBg).Padding(0, 1).Render(o))
		default:
			out = append(out, Muted.Padding(0, 1).Render(o))
		}
	}
	return strings.Join(out, " ")
}

// ChipAt returns the index of the chip drawn by Chips(options, ...) at
// column x, counted from the start of the row, and false if x is between
// or after the chips. Each chip is its text with a space on either side,
// and one space separates two chips.
func ChipAt(options []string, x int) (int, bool) {
	left := 0
	for i, o := range options {
		right := left + lipgloss.Width(o) + 2
		if x >= left && x < right {
			return i, true
		}
		left = right + 1
	}
	return 0, false
}

// Field draws one field of a wizard card: a mark when focused, the label
// in a fixed column, then the value (chips or a text input).
func Field(label string, focused bool, value string) string {
	mark := strings.Repeat(" ", FieldMarkWidth)
	style := Muted
	if focused {
		mark, style = Fg(Purple).Bold(true).Render("▸ "), Bold
	}
	return mark + style.Width(FieldLabelWidth).Render(label) + value
}

// SetupCard draws the wizard's card: a purple box of the given width with
// a bold title and the body below it.
func SetupCard(width int, title, body string) string {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Purple).
		Padding(1, 2).Width(width).Render(Bold.Render(title) + "\n\n" + body)
}
