package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/commands"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
)

// wheelRows is how many rows one turn of the mouse wheel scrolls.
const wheelRows = 3

// Update handles one message and returns the new model, plus a command for
// Bubble Tea to run in the background (or nil). See the package comment.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.rendered = nil // the events must be wrapped again at the new width

	case tea.KeyMsg:
		m, cmd = m.handleKey(msg)

	case tea.MouseMsg:
		m = m.handleMouse(msg)

	case EventMsg:
		m = m.addEvent(activity.Event(msg))

	case StatusMsg:
		m, cmd = m.handleStatus(loop.Status(msg))

	case LoopDoneMsg:
		m.loopDone = true // Run returns the loop's exit code itself
		if m.quitting {
			return m, tea.Quit // /quit is complete: the label is back
		}

	case tickMsg:
		m.now = m.cfg.Now()
		cmd = tick()
		if !m.fetching && m.now.Sub(m.lastFetch) >= factsInterval {
			cmd = tea.Batch(cmd, m.fetchFacts())
			m.fetching = true
		}

	case factsMsg:
		m = m.handleFacts(msg)

	case replyMsg:
		m = m.handleReply(msg)
	}
	m = m.sync()
	return m, cmd
}

// handleStatus takes in a new status from the loop. When a turn ends, the
// GitHub facts are refreshed: an agent has probably merged or closed
// something.
func (m Model) handleStatus(s loop.Status) (Model, tea.Cmd) {
	turnEnded := m.status.Phase == loop.PhaseWorking && s.Phase != loop.PhaseWorking && s.Phase != loop.PhaseUsageWait
	m.status = s
	if s.Phase == loop.PhaseHuman {
		m = m.addEvent(m.huddleEvent("Type /reply <text> to answer on the control issue; Huddle then sets state:pm-ready"))
	}
	if turnEnded && !m.fetching {
		m.fetching = true
		return m, m.fetchFacts()
	}
	return m, nil
}

// handleKey sends a key press to the focused element, and nowhere else.
func (m Model) handleKey(k tea.KeyMsg) (Model, tea.Cmd) {
	// Ctrl+C works everywhere and behaves like /quit. Pressed again while
	// Huddle is stopping, it quits at once.
	if k.Type == tea.KeyCtrlC {
		if m.quitting {
			return m, tea.Quit
		}
		return m.runQuit()
	}
	switch m.focus {
	case focusMenu:
		return m.menuKey(k)
	case focusActivity:
		return m.activityKey(k), nil
	default:
		return m.inputKey(k)
	}
}

// inputKey handles a key for the text field.
func (m Model) inputKey(k tea.KeyMsg) (Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyTab:
		m.focus = focusActivity
		return m, nil
	case tea.KeyEnter:
		return m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	// Text starting with "/" is a command: the menu opens. This also
	// catches pasted text, which arrives as one key with many runes.
	if strings.HasPrefix(m.input.Value(), "/") {
		m.focus, m.menuSel = focusMenu, 0
	}
	return m, cmd
}

// submit handles Enter in the text field. Plain text is never posted
// anywhere; until v3.2 brings the companion, it only gets an explanation.
func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	if text == "" {
		return m, nil
	}
	m = m.addEvent(m.youEvent(text))
	m = m.addEvent(m.huddleEvent("The companion arrives in v3.2; use /reply to answer the PM"))
	return m, nil
}

// menuKey handles a key while the slash-command menu is open. Typing still
// goes to the text field, where it filters the menu.
func (m Model) menuKey(k tea.KeyMsg) (Model, tea.Cmd) {
	matches := m.menuItems()
	switch k.Type {
	case tea.KeyEsc:
		m.input.Reset()
		m.focus = focusInput
		return m, nil
	case tea.KeyUp:
		m.menuSel = max(m.menuSel-1, 0)
		return m, nil
	case tea.KeyDown:
		m.menuSel = min(m.menuSel+1, max(len(matches)-1, 0))
		return m, nil
	case tea.KeyTab:
		if len(matches) > 0 {
			c := matches[m.menuSel]
			value := c.Name
			if c.Args {
				value += " "
			}
			m.input.SetValue(value)
			m.input.CursorEnd()
			m.menuSel = 0
		}
		return m, nil
	case tea.KeyEnter:
		return m.runTyped(matches)
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	if !strings.HasPrefix(m.input.Value(), "/") {
		m.focus = focusInput // the "/" was deleted: the menu closes
	}
	if m.input.Value() != before {
		m.menuSel = 0
	}
	return m, cmd
}

// activityKey handles a key for the Activity slot, depending on its view.
func (m Model) activityKey(k tea.KeyMsg) Model {
	switch k.Type {
	case tea.KeyTab:
		m.focus = focusInput
		return m
	case tea.KeyEsc:
		if m.view == viewList {
			m.focus = focusInput
		} else {
			m.view = viewList
		}
		return m
	}
	// "/" opens the menu from here too, as the help line says. A fast
	// typist's or a paste's "/status" arrives as one key with many runes.
	if k.Type == tea.KeyRunes && strings.HasPrefix(string(k.Runes), "/") {
		m.input.SetValue(string(k.Runes))
		m.input.CursorEnd()
		m.focus, m.menuSel = focusMenu, 0
		return m
	}

	switch m.view {
	case viewList, viewSelect:
		return m.listKey(k)
	case viewStatus:
		scroll(&m.statusVP, k)
	case viewLog:
		m = m.logKey(k)
	}
	return m
}

// listKey handles a key for the event list: ↑/↓ select a line, Enter opens
// the raw log at the selected line, PgUp/PgDn scroll.
func (m Model) listKey(k tea.KeyMsg) Model {
	switch k.Type {
	case tea.KeyUp, tea.KeyDown:
		if len(m.events) == 0 {
			return m
		}
		if m.view != viewSelect {
			// The first arrow selects the newest line.
			m.view, m.selected = viewSelect, len(m.events)-1
		} else if k.Type == tea.KeyUp {
			m.selected = max(m.selected-1, 0)
		} else {
			m.selected = min(m.selected+1, len(m.events)-1)
		}
		m = m.sync()
		m.list = scrollTo(m.list, m.eventLine(m.selected), m.eventLines(m.selected))
	case tea.KeyEnter:
		if m.view == viewSelect {
			if ref := m.events[m.selected].LogRef; !ref.IsZero() {
				m = m.openLog(ref)
			}
		}
	default:
		scroll(&m.list, k)
	}
	return m
}

// handleMouse handles the mouse: the wheel scrolls the pane under the
// pointer, a click focuses a pane or selects an Activity line.
func (m Model) handleMouse(msg tea.MouseMsg) Model {
	if m.tooSmall() {
		return m
	}
	l := m.layout()
	inActivity := msg.Y >= l.activityTop && msg.Y < l.activityTop+l.activityRows
	inSlot := msg.Y >= l.slotTop && msg.Y < l.slotTop+l.slotRows
	inInput := msg.Y >= l.inputTop && msg.Y < l.inputTop+inputRows

	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		rows := wheelRows
		if msg.Button == tea.MouseButtonWheelUp {
			rows = -wheelRows
		}
		switch {
		case inActivity:
			vp := m.activityViewport()
			if rows < 0 {
				vp.ScrollUp(-rows)
			} else {
				vp.ScrollDown(rows)
			}
		case inSlot && m.focus == focusMenu:
			last := max(len(m.menuItems())-1, 0)
			m.menuSel = min(max(m.menuSel+rows/wheelRows, 0), last)
		}

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m
		}
		switch {
		case inActivity:
			m.focus = focusActivity
			// Body rows start below the border and the title line.
			row := msg.Y - l.activityTop - 2
			if (m.view == viewList || m.view == viewSelect) && row >= 0 && row < m.list.Height {
				if i, ok := m.eventAt(m.list.YOffset + row); ok {
					m.view, m.selected = viewSelect, i
				}
			}
		case inInput:
			m.focus = focusInput
			if strings.HasPrefix(m.input.Value(), "/") {
				m.focus = focusMenu
			}
		}
	}
	return m
}

// activityViewport returns the viewport of the Activity slot's current
// view, so the caller can scroll it.
func (m *Model) activityViewport() *viewport.Model {
	switch m.view {
	case viewStatus:
		return &m.statusVP
	case viewLog:
		return &m.log.vp
	default:
		return &m.list
	}
}

// scroll scrolls vp for PgUp, PgDn, ↑ and ↓.
func scroll(vp *viewport.Model, k tea.KeyMsg) {
	switch k.Type {
	case tea.KeyPgUp:
		vp.PageUp()
	case tea.KeyPgDown:
		vp.PageDown()
	case tea.KeyUp:
		vp.ScrollUp(1)
	case tea.KeyDown:
		vp.ScrollDown(1)
	}
}

// scrollTo scrolls vp the least it can so that the n lines from line are
// visible.
func scrollTo(vp viewport.Model, line, n int) viewport.Model {
	switch {
	case line < vp.YOffset:
		vp.SetYOffset(line)
	case line+n > vp.YOffset+vp.Height:
		vp.SetYOffset(line + n - vp.Height)
	}
	return vp
}

// menuItems returns the commands the menu shows for what is typed.
func (m Model) menuItems() []commands.Command {
	return commands.Filter(commands.Visible(m.commandState()), m.input.Value())
}

// commandState is what the command visibility rules need to know.
func (m Model) commandState() commands.State {
	active := false
	switch m.status.Phase {
	case loop.PhaseStarting, loop.PhaseWorking, loop.PhaseUsageWait, loop.PhasePaused:
		active = true
	}
	return commands.State{
		AgentActive:   active && !m.loopDone,
		Paused:        m.paused,
		HumanRequired: m.status.Phase == loop.PhaseHuman,
	}
}
