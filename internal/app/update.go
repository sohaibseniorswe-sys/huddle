package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
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
		m, cmd = m.handleMouse(msg)

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

	case companionReplyMsg:
		m, cmd = m.handleCompanionReply(msg)

	case decisionMsg:
		m = m.handleDecision(msg)

	case spinner.TickMsg:
		// The spinner asks for its next frame with a tick message. Only
		// while the companion thinks: otherwise the ticks stop.
		if m.thinking {
			m.spin, cmd = m.spin.Update(msg)
		}
	}
	m = m.sync()
	return m, cmd
}

// handleStatus takes in a new status from the loop. When a turn ends, the
// GitHub facts are refreshed: an agent has probably merged or closed
// something.
func (m Model) handleStatus(s loop.Status) (Model, tea.Cmd) {
	turnEnded := m.status.Phase == loop.PhaseWorking && s.Phase != loop.PhaseWorking && s.Phase != loop.PhaseUsageWait
	humanNow := s.Phase == loop.PhaseHuman && m.status.Phase != loop.PhaseHuman
	m.status = s
	var cmds []tea.Cmd
	if humanNow {
		// The PM needs the human: the companion explains what it asks.
		m = m.addEvent(m.huddleEvent("Waiting for you. Talk to the companion; a decision you agree on goes to the PM"))
		var cmd tea.Cmd
		m, cmd = m.askAboutPMQuestion()
		cmds = append(cmds, cmd)
	}
	if turnEnded && !m.fetching {
		m.fetching = true
		cmds = append(cmds, m.fetchFacts())
	}
	return m, tea.Batch(cmds...)
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
	case focusCompanion:
		return m.companionKey(k), nil
	case focusCard:
		return m.cardKey(k)
	default:
		return m.inputKey(k)
	}
}

// inputKey handles a key for the text field.
func (m Model) inputKey(k tea.KeyMsg) (Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyTab:
		m.focus = m.nextFocus()
		return m, nil
	case tea.KeyEnter:
		return m.submit()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(asText(k))
	// Text starting with "/" is a command: the menu opens. This also
	// catches pasted text, which arrives as one key with many runes.
	if strings.HasPrefix(m.input.Value(), "/") {
		m.focus, m.menuSel = focusMenu, 0
	}
	return m, cmd
}

// asText marks a key message carrying several characters as pasted text.
// Bubble Tea delivers text that arrives quickly (a paste without bracketed
// paste, or keys sent by a tool such as tmux) as one message per word, and
// the text input's key bindings compare a message's text with key names:
// the word "right" would move the cursor instead of being typed. Pasted
// text never matches a binding.
func asText(k tea.KeyMsg) tea.KeyMsg {
	if k.Type == tea.KeyRunes && len(k.Runes) > 1 {
		k.Paste = true
	}
	return k
}

// submit handles Enter in the text field: plain text goes to the
// companion. It is never posted anywhere.
func (m Model) submit() (Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	if text == "" || m.quitting {
		return m, nil
	}
	m.input.Reset()
	return m.say(text, text)
}

// menuKey handles a key while the slash-command menu is open. Typing still
// goes to the text field, where it filters the menu.
func (m Model) menuKey(k tea.KeyMsg) (Model, tea.Cmd) {
	matches := m.menuItems()
	switch k.Type {
	case tea.KeyEsc:
		return m.closeMenu(), nil
	case tea.KeyUp:
		m.menuSel = max(m.menuSel-1, 0)
		return m, nil
	case tea.KeyDown:
		m.menuSel = min(m.menuSel+1, max(len(matches)-1, 0))
		return m, nil
	case tea.KeyTab:
		if len(matches) > 0 {
			m.input.SetValue(matches[m.menuSel].Name)
			m.input.CursorEnd()
			m.menuSel = 0
		}
		return m, nil
	case tea.KeyEnter:
		return m.runTyped(matches)
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(asText(k))
	if !strings.HasPrefix(m.input.Value(), "/") {
		m = m.closeMenu() // the "/" was deleted: the menu closes
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
		m.focus = m.nextFocus()
		return m
	case tea.KeyEsc:
		if m.view == viewList {
			m.focus = focusInput
		} else {
			m.view = viewList
		}
		return m
	}
	if m, ok := m.slashOpensMenu(k); ok {
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

// slashOpensMenu opens the menu when "/" is typed in a pane, as the help
// line says. A fast typist's or a paste's "/status" arrives as one key with
// many runes. Whatever was half-typed in the text field is put aside while
// the command is typed, and comes back when the menu closes: that is how a
// half-typed message survives /decision (Tab to a pane, then /decision).
func (m Model) slashOpensMenu(k tea.KeyMsg) (Model, bool) {
	if k.Type != tea.KeyRunes || !strings.HasPrefix(string(k.Runes), "/") {
		return m, false
	}
	m.stash = m.input.Value()
	m.input.SetValue(string(k.Runes))
	m.input.CursorEnd()
	m.focus, m.menuSel = focusMenu, 0
	return m, true
}

// closeMenu closes the menu: the text field gets back the text put aside
// when the menu opened, if any, and the keyboard.
func (m Model) closeMenu() Model {
	m.input.SetValue(m.stash)
	m.input.CursorEnd()
	m.stash = ""
	m.focus = focusInput
	return m
}

// nextFocus is where Tab moves the keyboard from the focused element:
// text field → Activity → Companion (when open) → text field.
func (m Model) nextFocus() focus {
	switch m.focus {
	case focusInput:
		return focusActivity
	case focusActivity:
		if m.companionOpen {
			return focusCompanion
		}
	}
	return focusInput
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
			if e := m.events[m.selected]; !e.LogRef.IsZero() {
				m = m.openLog(e)
			}
		}
	default:
		scroll(&m.list, k)
	}
	return m
}

// handleMouse handles the mouse: the wheel scrolls the pane under the
// pointer, a click focuses a pane, selects an Activity line or presses a
// card's button.
func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.tooSmall() {
		return m, nil
	}
	l := m.layout()
	inActivity := msg.Y >= l.activityTop && msg.Y < l.activityTop+l.activityRows
	inCompanion := msg.Y >= l.companionTop && msg.Y < l.companionTop+l.companionRows
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
			scrollBy(m.activityViewport(), rows)
		case inCompanion:
			scrollBy(&m.chatVP, rows)
		case inSlot && m.focus == focusMenu:
			last := max(len(m.menuItems())-1, 0)
			m.menuSel = min(max(m.menuSel+rows/wheelRows, 0), last)
		}

	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
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
		case inCompanion:
			m.focus = focusCompanion
		case inSlot && m.focus != focusMenu && m.card.kind != cardNone:
			// Find the button in the card as it is drawn now, before the
			// focus changes how its buttons look.
			b, ok := m.cardView(l.slotRows).ButtonAt(msg.X, msg.Y-l.slotTop)
			m.focus = focusCard
			if ok {
				m.card.sel = b
				return m.confirmCard()
			}
		case inInput:
			m.focus = focusInput
			if strings.HasPrefix(m.input.Value(), "/") {
				m.focus = focusMenu
			}
		}
	}
	return m, nil
}

// scrollBy scrolls vp down by rows, or up if rows is negative.
func scrollBy(vp *viewport.Model, rows int) {
	if rows < 0 {
		vp.ScrollUp(-rows)
	} else {
		vp.ScrollDown(rows)
	}
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
		CompanionOpen: m.companionOpen,
		Conversation:  m.conversation(),
		CardWaiting:   m.card.kind != cardNone,
		AgentActive:   active && !m.loopDone,
		Paused:        m.paused,
	}
}
