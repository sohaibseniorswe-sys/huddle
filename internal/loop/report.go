package loop

import (
	"fmt"
	"io"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// Reporter is told what the loop is doing. The run screen shows it; with
// --plain, PlainReporter prints it. The loop calls it from its own
// goroutine, so implementations must be safe to call from any goroutine.
type Reporter interface {
	// Event reports one line for the Activity pane.
	Event(activity.Event)
	// Status reports a change of what the loop is doing, for the badge in
	// the status bar.
	Status(Status)
}

// Phase is what the loop is doing.
type Phase int

// The phases of the loop, in the order a run usually meets them.
const (
	PhaseStarting  Phase = iota // the label has not been read yet
	PhaseNoLabel                // the control issue has no state:* label
	PhaseWorking                // Role's agent is running
	PhaseUsageWait              // Role hit a usage limit and waits until Until
	PhasePaused                 // the human paused the loop; Role's turn is next
	PhaseHuman                  // state:human-required: waiting for the human
	PhaseDone                   // state:done: the milestone is finished
)

// Status is what the loop is doing right now.
type Status struct {
	Phase Phase
	Role  session.Role // the agent working, waiting or next; "" if none
	Turn  int          // turns started by this Huddle so far
	Since time.Time    // when this status began
	Until time.Time    // PhaseUsageWait only: when the agent is resumed
}

// same reports whether s and t describe the same status, ignoring when
// each began. The loop sends a Status only when it changes.
func (s Status) same(t Status) bool {
	return s.Phase == t.Phase && s.Role == t.Role && s.Turn == t.Turn && s.Until.Equal(t.Until)
}

// PlainReporter prints the loop's own events as "huddle: ..." lines, exactly
// as Huddle v1 did, for huddle --plain. Agent events are not printed: with
// --plain the agents' raw output goes to the terminal instead.
type PlainReporter struct {
	Out io.Writer
}

// Event prints e if Huddle itself reported it.
func (r PlainReporter) Event(e activity.Event) {
	if e.Actor == activity.ActorHuddle {
		fmt.Fprintf(r.Out, "huddle: %s\n", e.Text)
	}
}

// Status prints nothing: every status change also comes as an event.
func (PlainReporter) Status(Status) {}
