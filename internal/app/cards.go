package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/commands"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// This file is the cards: a decision or an action the companion proposed,
// waiting in the slot above the text field until the human answers it.
//
// A card never takes the keyboard by itself. While it waits, the text field
// keeps the focus and the card's buttons are greyed out; Tab, /decision or
// a click moves the focus to the card. Only one card waits at a time: a new
// one replaces it.

// cardKind is what a card proposes.
type cardKind int

const (
	cardNone     cardKind = iota // no card waits
	cardDecision                 // a decision to post on the control issue
	cardAction                   // a Huddle action: pause, resume, status, log HH:MM
)

// The buttons. Agree is always the first.
const (
	buttonAgree = 0
	buttonNo    = 1
)

// decisionMarker opens every decision Huddle posts, so agents and people
// can find them. The PM's instructions tell it to act on these first.
const decisionMarker = "<!-- huddle:decision -->"

// card is the card waiting for an answer.
type card struct {
	kind   cardKind
	text   string // the decision, or the action ("pause", "log 22:52")
	reason string // a decision's one-line reason; may be ""
	sel    int    // the chosen button: buttonAgree or buttonNo
}

// noun names the card for the help line.
func (c card) noun() string {
	if c.kind == cardAction {
		return "action"
	}
	return "decision"
}

// describe says what the card is, for the companion's context block.
func (c card) describe() string {
	switch c.kind {
	case cardDecision:
		s := fmt.Sprintf("decision %q", c.text)
		if c.reason != "" {
			s += fmt.Sprintf(" (reason: %s)", c.reason)
		}
		return s + ", not answered yet"
	case cardAction:
		return fmt.Sprintf("action %q, not answered yet", c.text)
	}
	return ""
}

// actionText describes an action for its card: "pause after this turn".
func actionText(action string) string {
	switch {
	case action == "pause":
		return "pause after this turn"
	case action == "resume":
		return "resume: start the next turn now"
	case action == "status":
		return "show /status"
	case strings.HasPrefix(action, "log "):
		return "open the raw log at " + strings.TrimPrefix(action, "log ")
	}
	return action
}

// showCard puts c in the slot, replacing any card that waits. The focus
// stays where it is, unless it was on the old card.
func (m Model) showCard(c card) Model {
	m.card = c
	if m.focus == focusCard {
		m.focus = focusInput
	}
	return m.sync()
}

// clearCard removes the card and gives the keyboard back to the text field.
func (m Model) clearCard() Model {
	m.card = card{}
	if m.focus == focusCard {
		m.focus = focusInput
	}
	return m
}

// cardView returns the card as ui draws it in a slot rows high.
func (m Model) cardView(rows int) ui.Card {
	c := ui.Card{
		Title:    "DECISION",
		Accent:   ui.Green,
		Text:     m.card.text,
		Buttons:  [2]string{"Agree", "Keep talking"},
		Selected: m.card.sel,
		Focused:  m.focus == focusCard,
		Width:    m.width,
		Height:   rows,
	}
	if m.card.reason != "" {
		c.Note = "Reason: " + m.card.reason
	}
	if m.card.kind == cardAction {
		c.Title, c.Accent = "ACTION", ui.Purple
		c.Text = "Suggested action: " + actionText(m.card.text)
		c.Buttons = [2]string{"Agree", "Not now"}
	}
	return c
}

// cardKey handles a key while the card has the focus: ←/→ choose, Enter
// confirms, Esc goes back to the text field, whose half-typed text was
// never touched, Tab moves on to the next pane, and "/" opens the menu.
func (m Model) cardKey(k tea.KeyMsg) (Model, tea.Cmd) {
	// As in the panes, "/" opens the menu.
	if m, ok := m.slashOpensMenu(k); ok {
		return m, nil
	}
	switch k.Type {
	case tea.KeyLeft:
		m.card.sel = buttonAgree
	case tea.KeyRight:
		m.card.sel = buttonNo
	case tea.KeyEnter:
		return m.confirmCard()
	case tea.KeyEsc:
		m.focus = focusInput
	case tea.KeyTab:
		m.focus = m.nextFocus()
	}
	return m, nil
}

// confirmCard does what the chosen button says. The card goes away first,
// so a second Enter cannot post a decision twice.
func (m Model) confirmCard() (Model, tea.Cmd) {
	c := m.card
	m = m.clearCard()
	if c.sel == buttonNo {
		return m, nil // Keep talking or Not now: nothing happens anywhere
	}
	if c.kind == cardAction {
		return m.runAction(c.text)
	}
	m = m.addEvent(m.huddleEvent(fmt.Sprintf("Posting the decision on #%d…", m.cfg.Control.Number)))
	return m, m.postDecision(c)
}

// runAction runs an action the human agreed to.
func (m Model) runAction(action string) (Model, tea.Cmd) {
	m = m.addEvent(m.youEvent("Agreed: " + actionText(action)))
	switch {
	case action == "pause":
		return m.runCommand(commands.Pause, "")
	case action == "resume":
		return m.runCommand(commands.Resume, "")
	case action == "status":
		return m.runCommand(commands.Status, "")
	case strings.HasPrefix(action, "log "):
		return m.openLogAt(strings.TrimPrefix(action, "log ")), nil
	}
	return m, nil
}

// openLogAt opens the raw log at the Activity event closest to hhmm
// ("22:52"): the first with a log line at that minute, else the last one
// before it, else the end of the newest run.
func (m Model) openLogAt(hhmm string) Model {
	best := -1
	for i, e := range m.events {
		if e.LogRef.IsZero() {
			continue
		}
		at := e.Time.Format(eventTimeLayout)
		if at == hhmm {
			best = i
			break
		}
		if at < hhmm { // "15:04" strings sort like the times they show
			best = i
		}
	}
	if best < 0 {
		m = m.addEvent(m.huddleEvent("No agent output logged at " + hhmm + " in this run; showing the newest log"))
		return m.openLatestLog()
	}
	return m.openLog(m.events[best])
}

// decisionMsg is sent by the command postDecision returns, when GitHub has
// answered.
type decisionMsg struct {
	card   card  // the decision
	posted bool  // the comment is on the control issue
	handed bool  // state:pm-ready was set, so the PM acts now
	err    error // what went wrong, if anything
}

// decisionComment is the comment an agreed decision becomes.
func decisionComment(c card) string {
	body := decisionMarker + "\n**Decision** (agreed in Huddle)\n\n" + c.text + "\n"
	if c.reason != "" {
		body += "\nReason: " + c.reason + "\n"
	}
	return body
}

// postDecision returns the command that posts c on the control issue, as
// the human (it is their gh login). If the control issue says
// state:human-required, it also sets state:pm-ready so the PM acts now;
// otherwise the label stays and the PM reads the decision at its next turn.
func (m Model) postDecision(c card) tea.Cmd {
	control := m.cfg.Control
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
		defer cancel()
		if err := control.Comment(ctx, decisionComment(c)); err != nil {
			return decisionMsg{card: c, err: err}
		}
		state, err := control.State(ctx)
		if err != nil || state != gh.StateHumanRequired {
			return decisionMsg{card: c, posted: true, err: err}
		}
		if err := control.SetState(ctx, gh.StatePMReady); err != nil {
			return decisionMsg{card: c, posted: true, err: err}
		}
		return decisionMsg{card: c, posted: true, handed: true}
	}
}

// handleDecision reports how posting a decision went. A decision that
// could not be posted comes back as a card, so the human can try again.
func (m Model) handleDecision(msg decisionMsg) Model {
	number := m.cfg.Control.Number
	if !msg.posted {
		m = m.addEvent(m.huddleEvent("Could not post the decision: " + msg.err.Error()))
		if m.card.kind == cardNone {
			m = m.showCard(msg.card)
		}
		return m
	}
	m = m.addEvent(activity.Event{Time: m.cfg.Now(), Actor: activity.ActorYou, Text: fmt.Sprintf("Decision posted on #%d", number)})
	m = m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoDecision, Text: msg.card.text, Reason: msg.card.reason})
	switch {
	case msg.err != nil:
		m = m.addEvent(m.huddleEvent(fmt.Sprintf("The decision is posted, but Huddle could not check or set the label: %v. Set %s yourself if the PM should act now", msg.err, gh.StatePMReady)))
	case msg.handed:
		// Wake the loop, so the PM starts now instead of at its next poll.
		m.cfg.Controls.Wake()
		m = m.addEvent(m.huddleEvent(fmt.Sprintf("Set %s: the PM acts on the decision now", gh.StatePMReady)))
	default:
		m = m.addEvent(m.huddleEvent("The PM reads the decision at its next turn"))
	}
	return m
}
