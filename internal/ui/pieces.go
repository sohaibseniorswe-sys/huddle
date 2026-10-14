package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Sizes of the fixed parts of a pane.
const (
	// PaneChromeRows is the rows a pane uses besides its body: the top
	// border, the title line and the bottom border.
	PaneChromeRows = 3
	// PaneChromeCols is the columns a pane uses besides its body: a border
	// and a space of padding on each side.
	PaneChromeCols = 4
)

// menuNameWidth is the column width of a command name in the menu.
const menuNameWidth = 15

// StatusBar draws the first row of the run screen: the HUDDLE badge, the
// project info (repo, milestone, control issue) and, on the right, a badge
// saying what is happening, in badgeColor.
func StatusBar(width int, info, badge string, badgeColor lipgloss.Color) string {
	logo := lipgloss.NewStyle().Bold(true).Foreground(White).Background(Purple).Padding(0, 1).Render("HUDDLE")
	right := lipgloss.NewStyle().Bold(true).Foreground(Black).Background(badgeColor).Padding(0, 1).Render("● " + badge)
	room := width - lipgloss.Width(logo) - lipgloss.Width(right)
	middle := lipgloss.NewStyle().Foreground(Text).Background(BarBg).Padding(0, 1).
		Width(max(room, 0)).MaxWidth(max(room, 0)).Render(info)
	return logo + middle + right
}

// InfoLine draws the second row of the run screen from parts that are
// already styled, separated by dim dots.
func InfoLine(width int, parts []string) string {
	line := " " + strings.Join(parts, Muted.Render("  ·  "))
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// Pane is a bordered box with a title line, used for Activity, Status, the
// raw log and the Companion.
type Pane struct {
	Title   string
	Hint    string         // dim text on the right of the title line
	Body    string         // the content; lines beyond the pane are cut
	Width   int            // total width, borders included
	Height  int            // total height, borders and title included
	Focused bool           // purple border and "◆ focused"
	Accent  lipgloss.Color // colour of the title
}

// Render draws the pane at exactly p.Width × p.Height.
func (p Pane) Render() string {
	inner := p.Width - PaneChromeCols
	border := Faint
	if p.Focused {
		border = Purple
	}

	title := lipgloss.NewStyle().Foreground(p.Accent).Bold(true).Render(p.Title)
	if p.Focused {
		title += Fg(Purple).Render("  ◆ focused")
	}
	if p.Hint != "" {
		gap := inner - lipgloss.Width(title) - lipgloss.Width(p.Hint)
		title += strings.Repeat(" ", max(gap, 1)) + Muted.Render(p.Hint)
	}

	// Exactly Height-3 body rows: cut what does not fit, pad what is short.
	rows := p.Height - PaneChromeRows
	lines := strings.Split(p.Body, "\n")
	if len(lines) > rows {
		lines = lines[:max(rows, 0)]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	body := lipgloss.NewStyle().MaxWidth(inner).Render(title + "\n" + strings.Join(lines, "\n"))

	// In Lip Gloss, Width includes the padding but not the border.
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(p.Width - 2).Render(body)
}

// Input draws the text field: a bordered row with a coloured prefix
// ("companion ›" or "command ›") followed by field, the text input's own
// view.
func Input(width int, prefix string, prefixColor lipgloss.Color, field string, focused bool) string {
	border := Faint
	if focused {
		border = Purple
	}
	line := Fg(prefixColor).Bold(true).Render(prefix+" › ") + field
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(width - 2).Render(
		lipgloss.NewStyle().MaxWidth(width - PaneChromeCols).Render(line))
}

// InputAside draws the text field greyed out while another element (a
// card) has the keyboard, with a note that its half-typed text is kept.
func InputAside(width int, prefix, text string) string {
	line := Muted.Bold(true).Render(prefix + " › ")
	if text != "" {
		line += Muted.Render(text + "   (kept while you answer)")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Faint).
		Padding(0, 1).Width(width - 2).Render(
		lipgloss.NewStyle().MaxWidth(width - PaneChromeCols).Render(line))
}

// Help draws the help line from key and description pairs:
// Help(width, "enter", "send", "/", "commands").
func Help(width int, pairs ...string) string {
	var items []string
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, Key.Render(pairs[i])+" "+Muted.Render(pairs[i+1]))
	}
	line := " " + strings.Join(items, Muted.Render("  ·  "))
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

// MenuItem is one row of the slash-command menu.
type MenuItem struct {
	Name string
	Desc string
	Lit  bool // marked with an amber ●
}

// Menu draws the slash-command menu at exactly width × height, with the
// row selected highlighted. When the items do not fit, the menu scrolls to
// keep the selected row visible and its last row says how many more there
// are ("↓ 3 more").
func Menu(width, height int, items []MenuItem, selected int) string {
	rows := height - 2 // the top and bottom border
	first, count := 0, len(items)
	more := ""
	if count > rows {
		count = rows - 1 // one row for "↓ n more"
		first = min(max(selected-count+1, 0), len(items)-count)
		if below := len(items) - first - count; below > 0 {
			more = fmt.Sprintf(" ↓ %d more", below)
		} else {
			more = fmt.Sprintf(" ↑ %d more", first)
		}
	}

	var lines []string
	for i := first; i < first+count; i++ {
		lines = append(lines, menuRow(width-2, items[i], i == selected))
	}
	if more != "" {
		lines = append(lines, Muted.Render(more))
	}
	if len(items) == 0 {
		lines = append(lines, Muted.Render(" no matching command"))
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(Purple).
		Width(width - 2).Render(strings.Join(lines[:max(rows, 0)], "\n"))
}

func menuRow(width int, it MenuItem, selected bool) string {
	if selected {
		return lipgloss.NewStyle().Background(Purple).Foreground(White).Bold(true).Width(width).MaxWidth(width).
			Render(" " + fmt.Sprintf("%-*s", menuNameWidth, it.Name) + it.Desc)
	}
	name := Key.Render(fmt.Sprintf("%-*s", menuNameWidth, it.Name))
	if it.Lit {
		pad := max(menuNameWidth-lipgloss.Width(it.Name)-2, 1)
		name = Key.Render(it.Name) + Fg(Amber).Bold(true).Render(" ●") + strings.Repeat(" ", pad)
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(" " + name + Muted.Render(it.Desc))
}

// ActorStyle is the style of an actor's name in the Activity pane.
func ActorStyle(actor string) lipgloss.Style {
	c := Dim
	switch actor {
	case "codex", "dev":
		c = Blue
	case "claude", "pm":
		c = Pink
	case "you":
		c = Green
	}
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}
