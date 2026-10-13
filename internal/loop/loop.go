// Package loop is Huddle's watch loop (MVP_PLAN.md section 8): it reads the
// control issue's state label and starts whichever agent's turn it is. It
// never judges the agents' work and never stops on its own; it stops only on
// state:done, Ctrl+C (context cancel) or, with Once, after one turn.
package loop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/prompts"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// PollInterval is how often the control issue's label is read while no agent
// is running.
const PollInterval = 30 * time.Second

// UsageRetry is how often a usage-limited agent is retried when the CLI gave
// no usable reset time.
const UsageRetry = 15 * time.Minute

// usageSlack is added to a reset time before the agent is resumed.
const usageSlack = time.Minute

// Exit codes returned by Run.
const (
	ExitOK          = 0
	ExitInterrupted = 130
)

// Clock is the time source, faked in tests.
type Clock interface {
	Now() time.Time
	// Sleep waits for d, or returns ctx.Err() if ctx is canceled first.
	Sleep(ctx context.Context, d time.Duration) error
}

// Agents runs one agent turn.
type Agents interface {
	// Run runs role with prompt on stdin, resuming sessionID if it is not
	// empty. turn numbers the log file. The error is only for a run that
	// could not start; a non-zero exit or a cancel is reported in Result.
	Run(ctx context.Context, role session.Role, sessionID, prompt string, turn int) (Result, error)
}

// Result is the outcome of one agent run.
type Result struct {
	ExitCode     int
	Canceled     bool   // killed because ctx was canceled (Ctrl+C)
	SessionID    string // the run's session id; "" if none was found
	FinalMessage string // the agent's final message, for the handoff comment
	// Output is everything to check for usage-limit messages: the full log,
	// the final message and, for Claude Code, its JSON envelope as the last
	// line.
	Output string
}

// Sessions stores each agent's session id for the control issue.
// session.Store implements it.
type Sessions interface {
	Load() session.IDs
	Save(role session.Role, id string) error
	Forget(role session.Role) error
}

var _ Sessions = session.Store{}

// Loop is the watch loop for one control issue.
type Loop struct {
	Control   gh.Control
	Milestone string
	Agents    Agents
	Sessions  Sessions
	Clock     Clock
	Out       io.Writer // Huddle's own status lines
	Once      bool      // run a single turn, then exit

	turn int // turns started by this process, for log names
}

// Markers identify the comments Huddle itself posts on the control issue.
func handoffMarker(role session.Role) string {
	return fmt.Sprintf("<!-- huddle:auto-handoff role=%s -->", role)
}

func usageWaitMarker(role session.Role) string {
	return fmt.Sprintf("<!-- huddle:usage-wait role=%s -->", role)
}

func escalateMarker(role session.Role) string {
	return fmt.Sprintf("<!-- huddle:escalate role=%s -->", role)
}

// errInterrupted reports that ctx was canceled.
var errInterrupted = errors.New("interrupted")

// Run runs the loop until state:done (ExitOK), Ctrl+C (ExitInterrupted), or
// the end of the first turn with Once (ExitOK).
func (l *Loop) Run(ctx context.Context) int {
	hintShown := false // "add state:dev-ready" printed for the current no-label spell
	shownComment := "" // the human-required comment already printed
	for {
		state, err := l.state(ctx)
		if err != nil {
			return l.interrupted()
		}
		if state != gh.StateHumanRequired {
			shownComment = ""
		}
		if state != "" {
			hintShown = false
		}

		switch state {
		case gh.StateDevReady, gh.StateDevWorking, gh.StatePMReady, gh.StatePMWorking:
			role := session.Dev
			if state == gh.StatePMReady || state == gh.StatePMWorking {
				role = session.PM
			}
			if state == gh.StateDevWorking || state == gh.StatePMWorking {
				l.printf("%s found: a previous Huddle run was interrupted; restarting the %s", state, roleName(role))
			}
			if err := l.turnFor(ctx, role); err != nil {
				return l.interrupted()
			}
			if l.Once {
				l.printf("--once: turn finished, exiting")
				return ExitOK
			}
			continue // read the label again straight away

		case gh.StateDone:
			l.printf("%s: the milestone is finished. Final report:", gh.StateDone)
			l.printLatestComment(ctx)
			return ExitOK

		case gh.StateHumanRequired:
			if c, ok, err := l.Control.LatestComment(ctx); err != nil {
				l.printf("warning: %v", err)
			} else if key := commentKey(c, ok); key != shownComment {
				shownComment = key
				l.printf("%s: waiting for the human. Latest comment on #%d:", gh.StateHumanRequired, l.Control.Number)
				l.printComment(c, ok)
				l.printf("set state:dev-ready or state:pm-ready to continue")
			}

		case "":
			if !hintShown {
				hintShown = true
				l.printf("control issue #%d has no state:* label; add %s to start", l.Control.Number, gh.StateDevReady)
			}
		}

		if l.Clock.Sleep(ctx, PollInterval) != nil {
			return l.interrupted()
		}
	}
}

func (l *Loop) interrupted() int {
	l.printf("interrupted")
	return ExitInterrupted
}

// turnFor runs one turn for role (section 8.2), including the resume
// fallback (8.4), usage-limit waits and escalation (8.6) and anti-stall
// (8.3). It returns errInterrupted on Ctrl+C.
func (l *Loop) turnFor(ctx context.Context, role session.Role) error {
	l.turn++
	working, ready := workingLabel(role), readyLabel(role)
	if err := l.retry(ctx, "set "+working, func() error { return l.Control.SetState(ctx, working) }); err != nil {
		return err
	}
	prompt, err := prompts.Launch(prompts.Role(role), l.Milestone, l.Control.Number)
	if err != nil {
		return err // only for an unknown role
	}

	sessionID := l.Sessions.Load().Get(role)
	freshTried := false    // the resume fallback runs once per turn
	waitCommented := false // one usage-wait comment per wait
	for {
		if sessionID != "" {
			l.printf("turn %d: %s, resuming session %s", l.turn, roleName(role), sessionID)
		} else {
			l.printf("turn %d: %s, fresh session", l.turn, roleName(role))
		}
		res, err := l.Agents.Run(ctx, role, sessionID, prompt, l.turn)
		if err != nil {
			// Could not start: treat as a failed run with that error as output.
			l.printf("warning: %v", err)
			res = Result{ExitCode: -1, Canceled: ctx.Err() != nil, FinalMessage: err.Error(), Output: err.Error()}
		}
		if res.SessionID != "" {
			if err := l.Sessions.Save(role, res.SessionID); err != nil {
				l.printf("warning: save %s session: %v", role, err)
			}
		}
		if res.Canceled || ctx.Err() != nil {
			l.restore(ready)
			return errInterrupted
		}
		l.printf("turn %d: %s exited with code %d", l.turn, roleName(role), res.ExitCode)

		// Read the label again. Normalising it removes our *-working label if
		// the agent set another one next to it.
		var state string
		if err := l.retry(ctx, "read the label", func() (err error) { state, err = l.Control.State(ctx); return err }); err != nil {
			return err
		}
		if state != working && state != "" {
			l.printf("%s set %s", roleName(role), state)
			return nil
		}

		// The label is unchanged. A usage limit comes first: such a run
		// never forgets its session and never hands over.
		usage := l.usageLimit(role, res.Output)
		switch {
		case usage.Hit:
			if !waitCommented {
				waitCommented = true
				if err := l.retry(ctx, "post the usage-wait comment", func() error {
					return l.Control.Comment(ctx, usageWaitComment(role, usage))
				}); err != nil {
					return err
				}
			}
			d := UsageRetry
			if !usage.ResetAt.IsZero() {
				if until := usage.ResetAt.Add(usageSlack).Sub(l.Clock.Now()); until > 0 {
					d = until
				}
			}
			l.printf("%s hit a usage limit (%s); waiting %s until %s, then resuming",
				roleName(role), usage.Message, d.Round(time.Second), l.Clock.Now().Add(d).Format("2006-01-02 15:04 MST"))
			if l.Clock.Sleep(ctx, d) != nil {
				l.restore(ready)
				return errInterrupted
			}
			sessionID = l.Sessions.Load().Get(role)
			continue

		case usage.Escalate:
			l.printf("%s stopped by an account or credit error: %s", roleName(role), usage.Message)
			if err := l.retry(ctx, "post the escalation comment", func() error {
				return l.Control.Comment(ctx, escalateComment(role, usage))
			}); err != nil {
				return err
			}
			return l.retry(ctx, "set "+gh.StateHumanRequired, func() error {
				return l.Control.SetState(ctx, gh.StateHumanRequired)
			})
		}

		if sessionID != "" && res.ExitCode != 0 && !freshTried {
			freshTried = true
			l.printf("resumed %s session %s failed; forgetting it and running the turn again in a fresh session", role, sessionID)
			if err := l.Sessions.Forget(role); err != nil {
				l.printf("warning: forget %s session: %v", role, err)
			}
			sessionID = ""
			continue
		}

		// Anti-stall: hand the turn to the other agent.
		next := readyLabel(other(role))
		l.printf("%s ended its turn without setting a label; handing over with %s", roleName(role), next)
		if err := l.retry(ctx, "post the handoff comment", func() error {
			return l.Control.Comment(ctx, handoffComment(role, res))
		}); err != nil {
			return err
		}
		return l.retry(ctx, "set "+next, func() error { return l.Control.SetState(ctx, next) })
	}
}

func (l *Loop) usageLimit(role session.Role, output string) runner.UsageLimit {
	if role == session.PM {
		return runner.ClaudeUsageLimit(output, l.Clock.Now())
	}
	return runner.CodexUsageLimit(output, l.Clock.Now())
}

// state reads the normalised state label, retrying on errors.
func (l *Loop) state(ctx context.Context) (string, error) {
	var state string
	err := l.retry(ctx, "read the label", func() (err error) { state, err = l.Control.State(ctx); return err })
	return state, err
}

// retry runs op until it succeeds, waiting PollInterval after each failure:
// a GitHub hiccup must not stop the loop. It fails only if ctx is canceled.
func (l *Loop) retry(ctx context.Context, what string, op func() error) error {
	for {
		if ctx.Err() != nil {
			return errInterrupted
		}
		err := op()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return errInterrupted
		}
		l.printf("warning: %s: %v; retrying in %s", what, err, PollInterval)
		if l.Clock.Sleep(ctx, PollInterval) != nil {
			return errInterrupted
		}
	}
}

// restore puts back the ready label of an interrupted agent, so the next
// huddle resumes it. ctx is already canceled, so this gets its own.
func (l *Loop) restore(ready string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := l.Control.SetState(ctx, ready); err != nil {
		l.printf("warning: could not restore %s: %v", ready, err)
		return
	}
	l.printf("restored %s", ready)
}

func (l *Loop) printLatestComment(ctx context.Context) {
	c, ok, err := l.Control.LatestComment(ctx)
	if err != nil {
		l.printf("warning: %v", err)
		return
	}
	l.printComment(c, ok)
}

func (l *Loop) printComment(c gh.Comment, ok bool) {
	if !ok {
		fmt.Fprintln(l.Out, "(no comments)")
		return
	}
	fmt.Fprintf(l.Out, "--- %s %s\n%s\n---\n", c.Author, c.URL, strings.TrimSpace(c.Body))
}

func (l *Loop) printf(format string, a ...any) {
	fmt.Fprintf(l.Out, "huddle: "+format+"\n", a...)
}

// commentKey identifies a comment, to print a human-required comment once.
func commentKey(c gh.Comment, ok bool) string {
	if !ok {
		return "none"
	}
	return c.URL + "\x00" + c.CreatedAt.String() + "\x00" + c.Body
}

func handoffComment(role session.Role, res Result) string {
	msg := strings.TrimSpace(res.FinalMessage)
	if msg == "" {
		msg = "(no output)"
	}
	return fmt.Sprintf("%s\n**Huddle:** the %s ended its turn without setting a `state:*` label (exit code %d), so Huddle is handing the turn to the %s with `%s`.\n\nIts final output:\n\n%s\n",
		handoffMarker(role), roleName(role), res.ExitCode, roleName(other(role)), readyLabel(other(role)), fence(msg))
}

func usageWaitComment(role session.Role, u runner.UsageLimit) string {
	reset := "The CLI gave no reset time, so Huddle retries every 15 minutes."
	if !u.ResetAt.IsZero() {
		reset = fmt.Sprintf("It resets at %s; Huddle resumes the %s a minute after that.", u.ResetAt.Format("2006-01-02 15:04 MST"), roleName(role))
	}
	return fmt.Sprintf("%s\n**Huddle:** the %s hit a usage limit. %s The turn stays with the %s; no handoff.\n\n%s\n",
		usageWaitMarker(role), roleName(role), reset, roleName(role), fence(u.Message))
}

func escalateComment(role session.Role, u runner.UsageLimit) string {
	return fmt.Sprintf("%s\n**Huddle:** the %s was stopped by an account or credit error that only the human can fix (no wait will clear it):\n\n%s\n\nFix the account, then set `%s` to resume the %s's session.\n",
		escalateMarker(role), roleName(role), fence(u.Message), readyLabel(role), roleName(role))
}

// fence wraps s in a code fence longer than any run of backticks inside it.
func fence(s string) string {
	n, run := 3, 0
	for _, r := range s {
		if r == '`' {
			run++
			n = max(n, run+1)
		} else {
			run = 0
		}
	}
	f := strings.Repeat("`", n)
	return f + "\n" + s + "\n" + f
}

func roleName(role session.Role) string {
	if role == session.PM {
		return "PM (Claude Code)"
	}
	return "Dev (Codex)"
}

func other(role session.Role) session.Role {
	if role == session.PM {
		return session.Dev
	}
	return session.PM
}

func readyLabel(role session.Role) string {
	if role == session.PM {
		return gh.StatePMReady
	}
	return gh.StateDevReady
}

func workingLabel(role session.Role) string {
	if role == session.PM {
		return gh.StatePMWorking
	}
	return gh.StateDevWorking
}
