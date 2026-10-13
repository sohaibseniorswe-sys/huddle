package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// View draws the whole screen. Bubble Tea calls it after every Update.
func (m Model) View() string {
	if m.width == 0 {
		return "" // no tea.WindowSizeMsg yet
	}
	if m.tooSmall() {
		return tooSmallText
	}
	l := m.layout()
	parts := []string{m.projectPane(), m.activitySlot(l.activityRows)}
	if l.slotRows > 0 {
		parts = append(parts, ui.Menu(m.width, l.slotRows, m.menuRows(), m.menuSel))
	}
	parts = append(parts, m.inputView(), m.helpLine())
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).Height(m.height).MaxHeight(m.height).
		Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// tooSmall reports whether the terminal is below the minimum size.
func (m Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

// layout lays out the screen for what is open now. Only the menu can open
// the slot in v3.1; the Companion arrives in v3.2.
func (m Model) layout() layout {
	return computeLayout(m.height, false, m.focus == focusMenu)
}

// sync fits the scrolling areas to the layout and gives them their current
// content. Update calls it after every message, so View only has to draw.
func (m Model) sync() Model {
	if m.tooSmall() {
		return m
	}
	l := m.layout()
	width := m.width - ui.PaneChromeCols
	rows := max(l.activityRows-ui.PaneChromeRows, 1)
	m.list.Width, m.list.Height = width, rows
	m.statusVP.Width, m.statusVP.Height = width, rows
	m.log.vp.Width, m.log.vp.Height = width, rows
	// The text field scrolls sideways once the text is wider than this.
	m.input.Width = width - lipgloss.Width("companion › ") - 1

	m = m.renderEvents(width)
	var list string
	m, list = m.listContent(width)
	m.list.SetContent(list)
	m.statusVP.SetContent(m.statusContent())
	m.log.vp.SetContent(m.log.content(width))
	return m
}

// projectPane draws the status bar and the info line.
func (m Model) projectPane() string {
	badge, colour := m.badge()
	name := m.cfg.Repo[strings.LastIndex(m.cfg.Repo, "/")+1:]
	info := fmt.Sprintf("%s · %s · control #%d", name, m.cfg.Milestone, m.cfg.Control.Number)
	return ui.StatusBar(m.width, info, badge, colour) + "\n" + ui.InfoLine(m.width, m.infoParts())
}

// activitySlot draws the Activity slot in its current view.
func (m Model) activitySlot(rows int) string {
	p := ui.Pane{
		Title:   "Activity",
		Width:   m.width,
		Height:  rows,
		Focused: m.focus == focusActivity,
		Accent:  ui.Dim,
	}
	switch m.view {
	case viewList:
		p.Body = m.list.View()
	case viewSelect:
		p.Hint = "↑/↓ select · enter open"
		p.Body = m.list.View()
	case viewStatus:
		p.Title, p.Hint, p.Accent = "Status", "esc: back to activity", ui.Purple
		p.Body = m.statusVP.View()
	case viewLog:
		p.Title, p.Hint, p.Accent = m.log.title(), "[ ] prev/next run · esc: back", ui.Purple
		p.Body = m.log.vp.View()
	}
	return p.Render()
}

// menuRows returns the menu's rows for what is typed.
func (m Model) menuRows() []ui.MenuItem {
	var rows []ui.MenuItem
	for _, c := range m.menuItems() {
		rows = append(rows, ui.MenuItem{Name: c.Name, Desc: c.Desc, Lit: c.Lit})
	}
	return rows
}

// inputView draws the text field. Its prefix says where Enter sends the
// text: to the companion, or to the command menu once it starts with "/".
func (m Model) inputView() string {
	prefix, colour := "companion", ui.Amber
	if strings.HasPrefix(m.input.Value(), "/") {
		prefix, colour = "command", ui.Purple
	}
	focused := m.focus == focusInput || m.focus == focusMenu
	return ui.Input(m.width, prefix, colour, m.input.View(), focused)
}

// helpLine draws the keys that work right now.
func (m Model) helpLine() string {
	var pairs []string
	switch {
	case m.quitting:
		pairs = []string{"", "Stopping the agent and putting its label back…", "ctrl+c", "quit at once"}
	case m.focus == focusMenu:
		pairs = []string{"↑/↓", "choose", "tab", "complete", "enter", "run", "esc", "close menu"}
	case m.focus == focusInput:
		pairs = []string{"enter", "send", "/", "commands", "tab", "activity", "ctrl+c", "quit"}
	case m.view == viewSelect:
		pairs = []string{"↑/↓", "select line", "enter", "open raw log there", "tab", "switch pane", "esc", "stop selecting"}
	case m.view == viewStatus:
		pairs = []string{"esc", "back", "pgup/pgdn", "scroll", "/", "commands", "tab", "switch pane"}
	case m.view == viewLog:
		pairs = []string{"pgup/pgdn", "scroll", "[ ]", "prev/next run", "g/G", "start/end", "esc", "back"}
	default:
		pairs = []string{"↑/↓", "select line", "pgup/pgdn", "scroll", "tab", "switch pane", "esc", "back to typing"}
	}
	return ui.Help(m.width, pairs...)
}
