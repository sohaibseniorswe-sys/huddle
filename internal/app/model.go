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
package app

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
)

// tickInterval is how often the screen updates the clock (for the elapsed
// time) and checks whether the GitHub facts are due for a refresh.
const tickInterval = 10 * time.Second

// factsInterval is the longest the GitHub facts go without a refresh. They
// are also refreshed after every turn.
const factsInterval = 60 * time.Second

// placeholder is the text field's grey hint while it is empty.
const placeholder = "Type to talk to the companion, / for commands"

// Config is what the run screen needs to know about the run.
type Config struct {
	Repo      string     // the target repo, "owner/name"
	Milestone string     // the milestone being worked on
	Control   gh.Control // the control issue, for the facts and /reply
	// Controls pauses, resumes and wakes the loop.
	Controls *loop.Controls
	// Stop cancels the loop's context: the running agent is killed and the
	// loop puts its ready label back, then the loop ends.
	Stop    context.CancelFunc
	LogDir  string           // where the agents' raw logs are, ~/.huddle/logs
	RepoDir string           // the repo's local clone; /log shows only its runs
	Now     func() time.Time // the clock; time.Now outside tests
}

// focus is the element that receives the keys. Exactly one has it.
type focus int

const (
	focusInput    focus = iota // the text field (the default)
	focusMenu                  // the slash-command menu, while it is open
	focusActivity              // the Activity slot
)

// view is what the Activity slot shows. Every view uses the same rows.
type view int

const (
	viewList   view = iota // Activity events, newest at the bottom
	viewSelect             // the same list with one line highlighted
	viewStatus             // /status
	viewLog                // /log: an agent's raw output
)

// Model is the run screen's state. See the package comment for how Bubble
// Tea uses it.
type Model struct {
	cfg Config

	width, height int // the terminal size, from tea.WindowSizeMsg
	now           time.Time
	started       time.Time // when the run screen started, for "Elapsed"

	focus focus
	view  view

	input   textinput.Model // the text field
	menuSel int             // the selected row of the menu

	events     []activity.Event
	rendered   [][]string     // each event drawn as lines, cached; see activity.go
	lineStarts []int          // the list line where each event starts
	selected   int            // the highlighted event in viewSelect
	list       viewport.Model // scrolls the event list
	statusVP   viewport.Model // scrolls the /status view
	log        logView        // the /log view

	status loop.Status // what the loop last reported
	paused bool        // /pause was used and no /resume yet

	facts     gh.Facts // GitHub facts for the info line and /status
	factsErr  error    // the last refresh's error, if it failed
	fetching  bool     // a refresh is running
	lastFetch time.Time

	quitting bool // /quit: waiting for the loop to put its label back
	loopDone bool // the loop has ended
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

	now := cfg.Now()
	return Model{
		cfg:      cfg,
		now:      now,
		started:  now,
		input:    in,
		status:   loop.Status{Phase: loop.PhaseStarting, Since: now},
		fetching: true, // Init starts the first fetch
		log:      logView{vp: viewport.New(0, 0)},
		list:     viewport.New(0, 0),
		statusVP: viewport.New(0, 0),
	}
}

// Init is called once by Bubble Tea when the program starts. The commands
// it returns start the clock and the first GitHub refresh.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(), m.fetchFacts())
}

// tickMsg is sent by tick every tickInterval.
type tickMsg time.Time

// tick returns a command that sends a tickMsg after tickInterval. Update
// calls it again on every tick, which makes the ticks repeat.
func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
