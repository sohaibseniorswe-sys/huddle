package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Card sizes.
const (
	// cardMinBodyRows is the fewest rows inside a card's border: the title,
	// two rows of text and the buttons, as in docs/v3/screens/10-decision.
	cardMinBodyRows = 4
	// cardButtonGap is the space between two buttons.
	cardButtonGap = 2
)

// Card is a decision or action card: a question the human answers with
// one of two buttons. It is drawn in the slot above the text field.
type Card struct {
	Title    string         // "DECISION" or "ACTION"
	Accent   lipgloss.Color // the border and title colour
	Text     string         // the statement, wrapped to the card's width
	Note     string         // a dim line below the text, such as the reason; shown only if there is room
	Buttons  [2]string      // "Agree" and "Keep talking" or "Not now"
	Selected int            // the button ←/→ has chosen, 0 or 1
	// Focused is true when the card has the keyboard. Unfocused, its
	// buttons are greyed out and it says how to focus it.
	Focused bool
	Width   int // total width, borders included
	Height  int // the rows the card may use; it is padded to exactly this
}

// Render draws the card at exactly c.Width × c.Height.
func (c Card) Render() string {
	inner := c.Width - PaneChromeCols
	rows := c.bodyRows()
	lines := []string{c.titleLine(inner)}
	lines = append(lines, c.textLines(inner, rows)...)
	lines = append(lines, c.buttonsLine())

	border := lipgloss.RoundedBorder()
	if c.Focused {
		border = lipgloss.ThickBorder() // a heavier frame shows where the keys go
	}
	box := lipgloss.NewStyle().Border(border).BorderForeground(c.Accent).Padding(0, 1).
		Width(c.Width - 2).Height(min(cardMinBodyRows, rows)).
		Render(lipgloss.NewStyle().MaxWidth(inner).Render(strings.Join(lines, "\n")))
	return lipgloss.NewStyle().Width(c.Width).Height(c.Height).MaxHeight(c.Height).Render(box)
}

// bodyRows is how many rows fit inside the border.
func (c Card) bodyRows() int { return max(c.Height-2, 2) }

// titleLine is the title with, on the right, how to answer the card.
func (c Card) titleLine(inner int) string {
	title := Fg(c.Accent).Bold(true).Render(c.Title)
	hint := Key.Render("tab") + Muted.Render(" or ") + Key.Render("/decision") + Muted.Render(" to answer")
	if c.Focused {
		title += Fg(Purple).Render("  ◆ focused")
		hint = Muted.Render("←/→ choose · enter confirm · esc back to typing")
	}
	gap := inner - lipgloss.Width(title) - lipgloss.Width(hint)
	return title + strings.Repeat(" ", max(gap, 1)) + hint
}

// textLines wraps the text, and the note if there is room, into the rows
// left between the title and the buttons. Text that does not fit ends
// with "…".
func (c Card) textLines(inner, rows int) []string {
	room := rows - 2 // the title and the buttons
	text := strings.Split(Plain.Width(inner).Render(c.Text), "\n")
	if len(text) > room {
		text = text[:max(room, 0)]
		if room > 0 {
			last := strings.TrimRight(text[room-1], " ")
			text[room-1] = Plain.Render(truncate(last, inner-1) + "…")
		}
	}
	if c.Note != "" && len(text) < room {
		note := strings.Split(Muted.Width(inner).Render(c.Note), "\n")
		text = append(text, note[:min(len(note), room-len(text))]...)
	}
	return text
}

// truncate cuts s to at most w columns.
func truncate(s string, w int) string {
	return lipgloss.NewStyle().MaxWidth(max(w, 0)).Render(s)
}

// buttonsLine draws the two buttons.
func (c Card) buttonsLine() string {
	var out []string
	for i, label := range c.Buttons {
		out = append(out, c.button(i, label))
	}
	return strings.Join(out, strings.Repeat(" ", cardButtonGap))
}

// button draws button i: chosen (green, "▸"), not chosen, or greyed out
// while the card is not focused.
func (c Card) button(i int, label string) string {
	switch {
	case !c.Focused:
		return Muted.Background(BarBg).Padding(0, 2).Render(label)
	case i == c.Selected:
		return lipgloss.NewStyle().Bold(true).Foreground(Black).Background(Green).Padding(0, 2).Render("▸ " + label)
	default:
		return lipgloss.NewStyle().Foreground(Text).Background(BarBg).Padding(0, 2).Render(label)
	}
}

// ButtonAt returns the button drawn at column x and row y of the card (both
// counted from the card's top-left corner), for mouse clicks.
func (c Card) ButtonAt(x, y int) (int, bool) {
	inner := c.Width - PaneChromeCols
	// The buttons come right after the top border, the title and the text.
	row := 2 + len(c.textLines(inner, c.bodyRows()))
	if y != row {
		return 0, false
	}
	left := 2 // the border and the padding
	for i, label := range c.Buttons {
		w := lipgloss.Width(c.button(i, label))
		if x >= left && x < left+w {
			return i, true
		}
		left += w + cardButtonGap
	}
	return 0, false
}

// ChatLine is one entry of the companion conversation.
type ChatLine struct {
	Who  string // "you", "companion", "huddle" or "decision"
	Text string
}

// Chat draws the companion conversation at width, entries separated by an
// empty line, each starting with who wrote it in its colour.
func Chat(width int, lines []ChatLine) string {
	var out []string
	for _, l := range lines {
		var head, body string
		switch l.Who {
		case "you":
			head, body = Fg(Green).Bold(true).Render("you  "), Plain.Render(l.Text)
		case "companion":
			head, body = Fg(Amber).Bold(true).Render("companion  "), Plain.Render(l.Text)
		case "decision":
			head, body = Fg(Green).Bold(true).Render("decision  "), Plain.Render(l.Text)
		default:
			head, body = Muted.Bold(true).Render("huddle  "), Muted.Render(l.Text)
		}
		out = append(out, lipgloss.NewStyle().Width(width).Render(head+body))
	}
	return strings.Join(out, "\n\n")
}
