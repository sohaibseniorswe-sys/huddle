// Package app is the run screen: the full-screen terminal UI that shows a
// Huddle run while the watch loop works.
//
// # How Bubble Tea works
//
// The screen is a Bubble Tea program. Bubble Tea follows "The Elm
// Architecture", which works differently from the event listeners of Java
// Swing or Cocoa:
//
//   - All UI state lives in one value, the Model.
//   - Everything that happens arrives as a message (a tea.Msg): a key
//     press, a mouse click, a new terminal size, a tick of a timer, or a
//     message we send ourselves, such as an Activity event from the loop.
//   - Bubble Tea calls Update(msg) for each message, one at a time, on a
//     single goroutine. Update returns the new Model. Because only Update
//     ever changes the Model, no locks are needed.
//   - After each Update, Bubble Tea calls View(), which turns the Model
//     into a string, and draws that string on the terminal.
//   - Update may also return a tea.Cmd: a function Bubble Tea runs in the
//     background (for example a GitHub request), whose result comes back
//     later as another message. That is how slow work stays out of Update.
//
// Update and View have value receivers (m Model, not m *Model): Update
// works on its own copy of the Model and returns it, and View cannot
// change the Model at all.
//
// # How the loop talks to the screen
//
// The watch loop (package loop) runs on its own goroutine. It must never
// touch the Model directly: that would race with Update. Instead it calls
// a Reporter, and this package's reporter turns each report into a message
// with Program.Send. The message then reaches Update like a key press
// does. See run.go.
//
// The screen talks back to the loop through loop.Controls (pause, resume,
// wake), and stops it by canceling its context (/quit).
//
// # The companion
//
// The companion (package companion) answers the human in its own pane.
// Each message runs the companion's CLI inside a tea.Cmd, so it never
// blocks Update, and the loop, on its own goroutine, never waits for it.
// See companion.go.
package app

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// tickInterval is how often the screen updates the clock (for the elapsed
// time) and checks whether the GitHub facts are due for a refresh.
const tickInterval = 10 * time.Second

// factsInterval is the longest the GitHub facts go without a refresh. They
// are also refreshed after every turn.
const factsInterval = 60 * time.Second

// placeholder is the text field's grey hint while it is empty.
const placeholder = "Type to talk to the companion, / for commands"

// Companion answers the human's messages. companion.Agent implements it;
// tests use a fake.
type Companion interface {
	// Ask sends message, wrapped in the context block, and returns the
	// companion's reply. It blocks until the companion has answered.
	Ask(ctx context.Context, contextBlock, message string) (string, error)
}

// Config is what the run screen needs to know about the run.
type Config struct {
	Draft       *planning.Workspace // v4 planning store; nil retains legacy plan behavior
	LaneWarning string              // nonblocking shared-CLI usage warning
	ProjectID   string              // stable registry identity; independent of optional Repo
	Repo        string              // the target repo, "owner/name"
	Milestone   string              // the milestone being worked on
	Control     gh.Control          // the control issue, for the facts and decisions
	// Controls pauses, resumes and wakes the loop.
	Controls *loop.Controls
	// Stop cancels the loop's context: the running agent is killed and the
	// loop puts its ready label back, then the loop ends.
	Stop context.CancelFunc
	// Team is what plays each role, named in the badge, /status and
	// /settings.
	Team agents.Team
	// Settings is what /settings shows besides the fields above.
	Settings Settings
	LogDir   string           // where the agents' raw logs are, ~/.huddle/logs
	RepoDir  string           // the repo's local clone; /log shows only its runs
	Now      func() time.Time // the clock; time.Now outside tests

	// Companion answers the human; nil means there is none.
	Companion Companion
	// History is the companion conversation's file. A zero History (no
	// Path) keeps the conversation in memory only.
	History companion.History
	// CompanionCtx is the context every companion run gets. Run cancels it
	// on /quit and when the screen closes, which kills a running companion.
	// nil means context.Background().
	CompanionCtx context.Context
	// StopCompanion cancels CompanionCtx; nil means nothing to cancel.
	StopCompanion context.CancelFunc

	// Planning opens the screen in plan mode: the Companion pane open and
	// no milestone running until a plan is agreed. Milestone and Control
	// are then empty.
	Planning bool
	// First is the control issue to run first (not in plan mode).
	First gh.Issue
	// NewLoop returns the watch loop for a control issue. Run calls it for
	// First and for every queued milestone after it; Run sets the loop's
	// Report and Controls.
	NewLoop func(control gh.Issue) *loop.Loop
	// Queue is the milestone queue (docs/v3/issues/06-start-modes.md).
	Queue start.Queue
	// Send hands a message to the running program (Program.Send); Run
	// sets it. Background commands use it to report progress.
	Send func(tea.Msg)
	// Begin tells the loop goroutine to start the first queued milestone,
	// after a plan was agreed in plan mode; Run sets it.
	Begin func()
}

// focus is the element that receives the keys. Exactly one has it.
type focus int

const (
	focusInput     focus = iota // the text field (the default)
	focusMenu                   // the slash-command menu, while it is open
	focusActivity               // the Activity slot
	focusCompanion              // the Companion pane
	focusCard                   // the decision or action card, after /decision
)

// view is what the Activity slot shows. Every view uses the same rows.
type view int

const (
	viewList     view = iota // Activity events, newest at the bottom
	viewSelect               // the same list with one line highlighted
	viewStatus               // /status
	viewLog                  // /log: an agent's raw output
	viewSettings             // /settings, read-only
	viewPlan                 // a plan in full, from its card's Show all
)

// Model is the run screen's state. See the package comment for how Bubble
// Tea uses it.
type Model struct {
	selection      textSelection
	clipboardEpoch int
	clipboardFrame string // OSC 52 emitted in the next rendered frame only
	cfg            Config

	width, height int // the terminal size, from tea.WindowSizeMsg
	now           time.Time
	started       time.Time // when the run screen started, for "Elapsed"

	focus focus
	view  view

	input   textinput.Model // the text field
	menuSel int             // the selected row of the menu
	// stash is the text field's half-typed text while a command typed in
	// from a pane is in the field; see slashOpensMenu.
	stash string

	events     []activity.Event
	rendered   [][]string     // each event drawn as lines, cached; see activity.go
	lineStarts []int          // the list line where each event starts
	selected   int            // the highlighted event in viewSelect
	list       viewport.Model // scrolls the event list
	statusVP   viewport.Model // scrolls the /status view
	settingsVP viewport.Model // scrolls the /settings view
	// settings is the /settings view's text, drawn when it opens: it
	// reads the logs, which Update must not do on every message.
	settings string
	log      logView // the /log view

	status loop.Status // what the loop last reported
	paused bool        // /pause was used and no /resume yet

	facts     gh.Facts // GitHub facts for the info line and /status
	factsErr  error    // the last refresh's error, if it failed
	fetching  bool     // a refresh is running
	lastFetch time.Time

	quitting bool // /quit: waiting for the loop to put its label back
	loopDone bool // the loop has ended

	// The companion; see companion.go.
	companionOpen bool              // the Companion pane is shown
	chat          []companion.Entry // the conversation, oldest first
	chatVP        viewport.Model    // scrolls the conversation
	thinking      bool              // a companion run is going on
	spin          spinner.Model     // turns while thinking
	queue         []string          // messages sent while thinking, asked next
	queuePM       bool              // the next ask starts with the PM's question
	card          card              // the card waiting for an answer, if any
	historyErr    bool              // a history write failed and was reported

	// Plan mode and the queue; see plan.go and run.go.
	planning bool           // plan mode: no milestone runs yet
	planText string         // the plan Show all opened
	planVP   viewport.Model // scrolls it
	queued   []int          // the milestone queue, as last read
	finished bool           // the last milestone is done and nothing is queued
	toStart  bool           // a key was pressed after finished: back to the start menu
}

// New returns the run screen's model for cfg.
func New(cfg Config) Model {
	in := textinput.New()
	in.Prompt = "" // the "companion ›" prefix is drawn by ui.Input
	in.Placeholder = placeholder
	// A steady cursor: a blinking one would need a timer message twice a
	// second, for nothing.
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Focus()

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = ui.Fg(ui.Amber)

	now := cfg.Now()
	m := Model{
		cfg:        cfg,
		now:        now,
		started:    now,
		input:      in,
		status:     loop.Status{Phase: loop.PhaseStarting, Since: now},
		fetching:   !cfg.Planning, // Init starts the first fetch, unless planning
		log:        logView{vp: viewport.New(0, 0)},
		list:       viewport.New(0, 0),
		statusVP:   viewport.New(0, 0),
		settingsVP: viewport.New(0, 0),
		chatVP:     viewport.New(0, 0),
		planVP:     viewport.New(0, 0),
		spin:       spin,
		planning:   cfg.Planning,
	}
	m.queued = m.loadQueue()
	if cfg.Planning {
		// Plan mode is a conversation: the Companion pane is open from the
		// start.
		m.companionOpen = true
	}
	// The conversation of an earlier Huddle on this control issue comes
	// back. Reading a small file here, before the screen starts, is fine.
	if cfg.History.Path != "" {
		chat, err := cfg.History.Load()
		m.chat = chat
		if err != nil {
			m.chat = append(m.chat, companion.Entry{Time: now, Who: companion.WhoHuddle, Text: err.Error()})
		}
	}
	if cfg.LaneWarning != "" {
		m = m.addEvent(m.huddleEvent(cfg.LaneWarning))
	}
	return m
}

// Init is called once by Bubble Tea when the program starts. The commands
// it returns start the clock and the first GitHub refresh.
func (m Model) Init() tea.Cmd {
	if m.planning {
		return tick() // no milestone yet, so no GitHub facts to read
	}
	return tea.Batch(tick(), m.fetchFacts())
}

// tickMsg is sent by tick every tickInterval.
type tickMsg time.Time

// tick returns a command that sends a tickMsg after tickInterval. Update
// calls it again on every tick, which makes the ticks repeat.
func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
