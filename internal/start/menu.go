package start

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// This file is the start menu's Bubble Tea model. Like the run screen and
// the wizard, it keeps all its state in Model; Bubble Tea passes every key
// to Update, which returns the changed copy, and calls View to draw it.
// Everything slow (reading GitHub) happened before the menu opened, so
// this model never starts background work.

// Kind is what the human chose in the start menu.
type Kind int

const (
	KindQuit   Kind = iota // quit Huddle (also Esc, q and Ctrl+C)
	KindPlan               // plan new milestones with the companion
	KindRun                // run an existing milestone
	KindResume             // resume the interrupted run
)

// Choice is what the start menu returns.
type Choice struct {
	Kind Kind
	// Milestone is the milestone to run (KindRun).
	Milestone MilestoneEntry
	// Message is the optional first message for the Dev (KindRun); ""
	// keeps the default prompt.
	Message string
	// Resume is the control issue to resume (KindResume).
	Resume gh.Issue
}

// page is what the menu shows.
type page int

const (
	pageMenu     page = iota // the five options (screen 14)
	pagePick                 // the open milestones (screen 15)
	pageMessage              // the first message for the Dev, after picking
	pageSettings             // the read-only settings
)

// option is one row of the main menu.
type option struct {
	name, note string
	kind       Kind
	settings   bool // the Settings row, which opens a page instead of returning
	pick       bool // the Run an existing milestone row
}

// The menu's texts.
const (
	menuTitle    = "What do you want to do?"
	menuFoot     = "Planned milestones run one after another, each with its own control issue."
	pickTitle    = "Which milestone should Huddle run?"
	messageTitle = "First message for the Dev"
)

// pickFoot is the picker's footer, as on screen 15.
var pickFoot = "Optional: type a first message for the Dev after choosing, or press enter\nto start with \"" + DefaultPrompt + "\""

// Model is the start menu.
type Model struct {
	facts    Facts
	settings string // the settings page's text, drawn by the caller
	width    int
	height   int
	page     page
	sel      int // the selected row of the current page
	picked   int // the milestone chosen on pagePick
	input    textinput.Model
	choice   Choice
}

// New returns the start menu for facts. settings is the text of the
// read-only settings page.
func New(facts Facts, settings string) Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = DefaultPrompt
	in.Cursor.SetMode(cursor.CursorStatic) // no blink timer needed
	return Model{facts: facts, settings: settings, input: in}
}

// Choice returns what the human chose; KindQuit until they chose.
func (m Model) Choice() Choice { return m.choice }

// options returns the main menu's rows: only those that make sense now.
func (m Model) options() []option {
	opts := []option{{name: "Plan new milestones with the companion", note: "talk, agree, Huddle creates them", kind: KindPlan}}
	if n := len(m.facts.Milestones); n > 0 {
		note := "pick one of " + plural(n, "open milestone")
		if n == 1 {
			note = "pick the open milestone"
		}
		opts = append(opts, option{name: "Run an existing milestone", note: note, kind: KindRun, pick: true})
	}
	if r := m.facts.Resume; r != nil {
		opts = append(opts, option{
			name: "Resume " + r.Milestone,
			note: fmt.Sprintf("control #%d · %s", r.Number, stateNote(ResumeState(*r))),
			kind: KindResume,
		})
	}
	return append(opts,
		option{name: "Settings", note: "read-only", settings: true},
		option{name: "Quit", kind: KindQuit},
	)
}

// stateNote says in words where an interrupted run stopped.
func stateNote(state string) string {
	switch state {
	case gh.StateDevWorking:
		return "Dev working when stopped"
	case gh.StateDevReady:
		return "Dev next"
	case gh.StatePMWorking:
		return "PM working when stopped"
	case gh.StatePMReady:
		return "PM next"
	case gh.StateHumanRequired:
		return "waiting for you"
	}
	return state
}

// milestoneNote describes a milestone in the picker:
// "4 issues · 1 closed · control #81".
func milestoneNote(e MilestoneEntry) string {
	parts := []string{plural(e.Issues, "issue")}
	if e.Closed > 0 {
		parts = append(parts, fmt.Sprintf("%d closed", e.Closed))
	}
	if e.Control != 0 {
		parts = append(parts, fmt.Sprintf("control #%d", e.Control))
	} else {
		parts = append(parts, "control issue created for you")
	}
	return strings.Join(parts, " · ")
}

// Init is called once when the program starts. The menu needs no timer
// and no background work.
func (m Model) Init() tea.Cmd { return nil }

// Update handles one message. See the package comment of internal/app for
// how Bubble Tea's Update works.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey handles a key on the current page.
func (m Model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type == tea.KeyCtrlC {
		return m.finish(Choice{Kind: KindQuit})
	}
	switch m.page {
	case pageMessage:
		return m.messageKey(k)
	case pageSettings:
		if k.Type == tea.KeyEsc || k.String() == "q" || k.Type == tea.KeyEnter {
			m.page, m.sel = pageMenu, m.settingsRow()
		}
		return m, nil
	}

	rows := len(m.options())
	if m.page == pagePick {
		rows = len(m.facts.Milestones)
	}
	switch {
	case k.Type == tea.KeyUp:
		m.sel = max(m.sel-1, 0)
	case k.Type == tea.KeyDown:
		m.sel = min(m.sel+1, rows-1)
	case k.Type == tea.KeyEnter:
		return m.open()
	case k.Type == tea.KeyEsc || k.String() == "q":
		if m.page == pagePick {
			m.page, m.sel = pageMenu, m.pickRow()
			return m, nil
		}
		return m.finish(Choice{Kind: KindQuit})
	}
	return m, nil
}

// open acts on Enter in the menu or the picker.
func (m Model) open() (tea.Model, tea.Cmd) {
	if m.page == pagePick {
		m.picked, m.page = m.sel, pageMessage
		m.input.Reset()
		m.input.Focus()
		return m, nil
	}
	o := m.options()[m.sel]
	switch {
	case o.settings:
		m.page = pageSettings
		return m, nil
	case o.pick:
		m.page, m.sel = pagePick, 0
		return m, nil
	case o.kind == KindResume:
		return m.finish(Choice{Kind: KindResume, Resume: *m.facts.Resume})
	}
	return m.finish(Choice{Kind: o.kind})
}

// messageKey handles a key while the first message for the Dev is typed:
// Enter starts the milestone, Esc goes back to the picker.
func (m Model) messageKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.Type {
	case tea.KeyEnter:
		return m.finish(Choice{
			Kind:      KindRun,
			Milestone: m.facts.Milestones[m.picked],
			Message:   strings.TrimSpace(m.input.Value()),
		})
	case tea.KeyEsc:
		m.page, m.sel = pagePick, m.picked
		m.input.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(k)
	return m, cmd
}

// finish records the choice and ends the program. tea.Quit is a command
// that tells Bubble Tea to stop after this Update.
func (m Model) finish(c Choice) (tea.Model, tea.Cmd) {
	m.choice = c
	return m, tea.Quit
}

// settingsRow and pickRow are the menu rows Esc goes back to.
func (m Model) settingsRow() int { return m.rowWhere(func(o option) bool { return o.settings }) }
func (m Model) pickRow() int     { return m.rowWhere(func(o option) bool { return o.pick }) }

// rowWhere returns the first menu row for which match is true, or 0.
func (m Model) rowWhere(match func(option) bool) int {
	for i, o := range m.options() {
		if match(o) {
			return i
		}
	}
	return 0
}
