// Package companion is the third agent of a Huddle run: the one the human
// talks to on the run screen while the Dev and PM work.
//
// The companion knows everything about the run but never changes anything.
// Each message the human sends becomes one print-mode run of Claude Code
// (the PM's CLI) with read-only tools, in a session of its own that later
// messages resume. Huddle wraps every message in a context block (see
// Context) so the companion knows what the human is looking at.
//
// When the human and the companion agree on something the PM must act on,
// the companion writes it in <decision> tags. Parse finds it, the run
// screen shows it as a card, and only when the human presses Agree does
// Huddle post it on the control issue. The conversation is kept on disk
// (see History), so it survives a restart.
package companion

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

//go:embed instructions.md
var instructions string

// Instructions returns the companion's instructions for milestone. They
// open the first message of every new session.
func Instructions(milestone string) string {
	return strings.ReplaceAll(strings.TrimRight(instructions, "\n"), "{{milestone}}", milestone)
}

// humanHeader introduces the human's own words after the context block.
const humanHeader = "THE HUMAN SAYS:"

// Agent runs the companion. Its zero value is not usable: set every field.
type Agent struct {
	Dir       string        // the repo's local clone; the companion runs there
	Home      string        // the home directory, for ~/.huddle/logs/companion
	Milestone string        // filled into the instructions
	Sessions  session.Store // the companion's session file (session.NewCompanion)
	// Run runs one agent process. It is runner.Run outside tests.
	Run func(ctx context.Context, spec runner.Spec) (runner.Result, error)
	// Now is the clock, for log file names. It is time.Now outside tests.
	Now func() time.Time
}

// Ask sends one message to the companion and returns its reply, tags and
// all. contextBlock is the HUDDLE CONTEXT that Huddle built for this
// message; message is what the human typed.
//
// The first message starts a fresh session and carries the instructions;
// later ones resume it. If a resumed run fails without even printing its
// JSON envelope (as Claude Code does for a session that no longer exists),
// Ask forgets the session and asks again in a fresh one, once. A run that
// answers with an error (a usage limit, say) keeps its session.
//
// Ask blocks until the companion has answered, which takes seconds to
// minutes, so the run screen calls it from a tea.Cmd, never from Update.
// Canceling ctx kills the companion's process.
func (a Agent) Ask(ctx context.Context, contextBlock, message string) (string, error) {
	id := a.Sessions.Load().Get(session.Companion)
	reply, err := a.ask(ctx, id, contextBlock, message)
	if errors.Is(err, errNoEnvelope) && id != "" && ctx.Err() == nil {
		// The resumed session failed: start over in a fresh one.
		if ferr := a.Sessions.Forget(session.Companion); ferr != nil {
			return "", fmt.Errorf("forget the companion's session: %w", ferr)
		}
		reply, err = a.ask(ctx, "", contextBlock, message)
	}
	return reply, err
}

// ask runs one message in session id ("" for a fresh session) and saves
// the session id the companion reports.
func (a Agent) ask(ctx context.Context, id, contextBlock, message string) (string, error) {
	prompt := contextBlock + "\n\n" + humanHeader + "\n" + message + "\n"
	if id == "" {
		prompt = Instructions(a.Milestone) + "\n\n" + prompt
	}
	spec := runner.Companion(a.Dir, prompt, id)
	spec.LogPath = LogPath(a.Home, a.Now())

	res, err := a.Run(ctx, spec)
	if err != nil {
		return "", err
	}
	if res.Canceled || ctx.Err() != nil {
		return "", errors.New("the companion was stopped")
	}
	env, err := runner.ParseClaude(res.Stdout)
	if err != nil {
		return "", fmt.Errorf("%w (exit code %d): %s", errNoEnvelope, res.ExitCode, lastLine(res.Tail))
	}
	if env.SessionID != "" {
		if err := a.Sessions.Save(session.Companion, env.SessionID); err != nil {
			return "", fmt.Errorf("save the companion's session: %w", err)
		}
	}
	if env.IsError || res.ExitCode != 0 {
		return "", fmt.Errorf("companion failed (%s): %s", env.Subtype, lastLine(env.Result))
	}
	return strings.TrimSpace(env.Result), nil
}

// errNoEnvelope reports a run that printed no JSON envelope.
var errNoEnvelope = errors.New("the companion gave no answer")

// LogPath returns where one companion run's raw output is logged:
// <home>/.huddle/logs/companion/<timestamp>-companion.log. They are kept
// apart from the Dev and PM logs, which /log lists.
func LogPath(home string, t time.Time) string {
	return filepath.Join(runner.LogDir(home), "companion", t.Format("20060102-150405.000")+"-companion.log")
}

// lastLine returns the last non-empty line of s, for short error messages.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
