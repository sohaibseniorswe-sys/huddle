// Package activity turns what happens during a Huddle run into Events: the
// short lines shown in the Activity pane of the run screen.
//
// Events come from two places. Huddle's watch loop reports its own steps
// (turn started, label changed, usage wait) as events with the actor
// "huddle". The agents' raw output is turned into events by the parsers in
// this package (Codex and Claude), which read the output line by line while
// the agent runs. Each agent event remembers where its line is in the raw
// log file (a LogRef), so the /log view can open the log at that moment.
package activity

import "time"

// The actors an Event can have. The UI gives each its own colour.
const (
	ActorCodex  = "codex"  // output of the Codex CLI
	ActorClaude = "claude" // output of the Claude Code CLI
	ActorPM     = "pm"
	ActorDev    = "dev"
	ActorHuddle = "huddle" // Huddle itself
	ActorYou    = "you"    // the human, for example a slash command
)

// Event is one line in the Activity pane.
type Event struct {
	Time  time.Time
	Actor string // one of the Actor constants
	// Text is one short line, e.g. "Merged PR #71, closed #61". The only
	// longer texts are comments Huddle quotes from the control issue, which
	// keep their line breaks.
	Text   string
	LogRef LogRef // where this moment is in the raw log, if anywhere
}

// LogRef points at a line in an agent's raw log file.
type LogRef struct {
	Path   string // the log file; "" means the event has no place in a log
	Offset int64  // byte offset where the line starts in that file
}

// IsZero reports whether r points nowhere.
func (r LogRef) IsZero() bool { return r.Path == "" }
