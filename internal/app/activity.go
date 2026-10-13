package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// Column widths of the event list. A line looks like
//
//	"  22:41  codex   Merged PR #71, closed #61"
//
// and a long text wraps under its own column.
const (
	eventTimeLayout = "15:04"
	actorWidth      = 7
	// eventHeadWidth is the width of everything before the text.
	eventHeadWidth = 2 + len(eventTimeLayout) + 2 + actorWidth + 1
)

// emptyActivity is shown before the first event arrives.
const emptyActivity = "Nothing yet. What Huddle and the agents do appears here."

// addEvent appends e to the Activity list. The list keeps following new
// events while it is scrolled to the bottom; scrolled up, it stays put.
func (m Model) addEvent(e activity.Event) Model {
	follow := m.view == viewList && m.list.AtBottom()
	m.events = append(m.events, e)
	m = m.sync()
	if follow {
		m.list.GotoBottom()
	}
	// A raw log being watched at its end follows the agent's new output.
	if m.view == viewLog && e.LogRef.Path == m.log.path() && m.log.vp.AtBottom() {
		m.log = m.log.reloadEnd()
		m = m.sync()
		m.log.vp.GotoBottom()
	}
	return m
}

// huddleEvent returns an event from Huddle itself, dated now.
func (m Model) huddleEvent(text string) activity.Event {
	return activity.Event{Time: m.cfg.Now(), Actor: activity.ActorHuddle, Text: text}
}

// youEvent returns an event for something the human did, dated now.
func (m Model) youEvent(text string) activity.Event {
	return activity.Event{Time: m.cfg.Now(), Actor: activity.ActorYou, Text: text}
}

// renderEvents draws the events that are not drawn yet, at the current
// width. Each event is drawn once and kept in m.rendered: wrapping
// thousands of events again on every key press would be slow. A new
// terminal width clears m.rendered (see Update).
func (m Model) renderEvents(width int) Model {
	for i := len(m.rendered); i < len(m.events); i++ {
		m.rendered = append(m.rendered, renderEvent(m.events[i], width))
	}
	return m
}

// renderEvent draws one event as lines of exactly width columns.
func renderEvent(e activity.Event, width int) []string {
	head := "  " + ui.Muted.Render(e.Time.Format(eventTimeLayout)+"  ") +
		ui.ActorStyle(e.Actor).Width(actorWidth).Render(e.Actor) + " "
	body := ui.Plain.Width(max(width-eventHeadWidth, 1)).Render(e.Text)
	return strings.Split(lipgloss.JoinHorizontal(lipgloss.Top, head, body), "\n")
}

// renderSelected draws the selected event: highlighted across the whole
// width, with a hint if it has a raw log to open.
func renderSelected(e activity.Event, width int) []string {
	row := "▸ " + e.Time.Format(eventTimeLayout) + "  " + fmt.Sprintf("%-*s", actorWidth, e.Actor) + " " + e.Text
	if !e.LogRef.IsZero() {
		row += "   (enter: open raw log here)"
	}
	return strings.Split(lipgloss.NewStyle().Background(ui.SelectBg).Foreground(ui.White).Bold(true).
		Width(width).Render(row), "\n")
}

// listContent joins the drawn events into the list's content, and records
// in m.lineStarts the line where each event starts.
func (m Model) listContent(width int) (Model, string) {
	if len(m.events) == 0 {
		m.lineStarts = nil
		return m, ui.Muted.Render(emptyActivity)
	}
	var lines []string
	m.lineStarts = make([]int, 0, len(m.events))
	for i, r := range m.rendered {
		m.lineStarts = append(m.lineStarts, len(lines))
		if m.view == viewSelect && i == m.selected {
			r = renderSelected(m.events[i], width)
		}
		lines = append(lines, r...)
	}
	return m, strings.Join(lines, "\n")
}

// eventLine returns the line of the list where event i starts.
func (m Model) eventLine(i int) int {
	if i < 0 || i >= len(m.lineStarts) {
		return 0
	}
	return m.lineStarts[i]
}

// eventLines returns how many lines event i takes in the list.
func (m Model) eventLines(i int) int {
	if i+1 < len(m.lineStarts) {
		return m.lineStarts[i+1] - m.lineStarts[i]
	}
	return max(m.list.TotalLineCount()-m.eventLine(i), 1)
}

// eventAt returns the event drawn on line of the list.
func (m Model) eventAt(line int) (int, bool) {
	if len(m.lineStarts) == 0 || line < 0 || line >= m.list.TotalLineCount() {
		return 0, false
	}
	// The last event that starts at or before line.
	i := sort.SearchInts(m.lineStarts, line+1) - 1
	return i, i >= 0
}
