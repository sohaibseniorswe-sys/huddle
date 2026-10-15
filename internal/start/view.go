package start

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// cardMargin is the space left of the card; the card is as wide as the
// screen less twice this and its border.
const cardMargin = 2

// View draws the start menu. Bubble Tea calls it after every Update.
func (m Model) View() string {
	if m.width == 0 {
		return "" // no tea.WindowSizeMsg yet
	}
	name := m.facts.Repo[strings.LastIndex(m.facts.Repo, "/")+1:]
	bar := ui.StartBar(m.width, name+" · "+m.facts.Dir)
	card := lipgloss.NewStyle().MarginLeft(cardMargin).Render(ui.StartCard(m.width-6, m.title(), m.body(), m.foot()))
	screen := lipgloss.NewStyle().Width(m.width).Height(m.height - 1).MaxHeight(m.height - 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, bar, "", card))
	return screen + "\n" + m.help()
}

// title is the card's title for the current page.
func (m Model) title() string {
	switch m.page {
	case pagePick:
		return pickTitle
	case pageMessage:
		return messageTitle + " · " + m.facts.Milestones[m.picked].Title
	case pageSettings:
		return "Settings"
	}
	return menuTitle
}

// body is the card's main part for the current page.
func (m Model) body() string {
	switch m.page {
	case pagePick:
		var rows []string
		for i, e := range m.facts.Milestones {
			rows = append(rows, ui.StartOption(e.Title, milestoneNote(e), i == m.sel))
		}
		return strings.Join(rows, "\n\n")
	case pageMessage:
		e := m.facts.Milestones[m.picked]
		return ui.Muted.Render(milestoneNote(e)) + "\n\n" + ui.Fg(ui.Purple).Bold(true).Render("› ") + m.input.View()
	case pageSettings:
		return m.settings
	}
	var rows []string
	for i, o := range m.options() {
		rows = append(rows, ui.StartOption(o.name, o.note, i == m.sel))
	}
	return strings.Join(rows, "\n\n")
}

// foot is the card's grey footer for the current page.
func (m Model) foot() string {
	switch m.page {
	case pagePick:
		return pickFoot
	case pageMessage:
		return "The message becomes the First Dev task in the control issue. Leave it empty\nfor \"" + DefaultPrompt + "\""
	case pageSettings:
		return ""
	}
	return menuFoot
}

// help is the bottom row: the keys that work on this page.
func (m Model) help() string {
	switch m.page {
	case pagePick:
		return ui.Help(m.width, "↑/↓", "choose", "enter", "start", "esc", "back")
	case pageMessage:
		return ui.Help(m.width, "enter", "start the milestone", "esc", "back")
	case pageSettings:
		return ui.Help(m.width, "esc", "back")
	}
	return ui.Help(m.width, "↑/↓", "choose", "enter", "open", "q", "quit")
}
