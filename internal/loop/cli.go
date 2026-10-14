package loop

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// RealClock is the wall clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CLIAgents runs the real agent CLIs in the repo root, logging their
// output under Home/.huddle/logs. Team says which CLI, model and effort
// plays the Dev and the PM. While an agent runs, its output is turned into
// Activity events (see package activity).
type CLIAgents struct {
	Dir    string      // repo root
	Home   string      // home directory, for ~/.huddle/logs
	Team   agents.Team // what plays each role
	Stream io.Writer   // live raw output: os.Stdout with --plain, nil for the run screen
}

// lineParser turns an agent's output lines into Activity events. The
// activity package has one per CLI.
type lineParser interface {
	Line(runner.Line)
	Flush()
}

func (a CLIAgents) Run(ctx context.Context, role session.Role, sessionID, prompt string, turn int, events func(activity.Event)) (Result, error) {
	choice := a.Team.For(role)
	logPath := runner.LogPath(a.Home, time.Now(), turn, string(role))
	lastMessage := strings.TrimSuffix(logPath, ".log") + ".last"
	spec := agents.Command(role, choice, sessionID, lastMessage)
	spec.Dir = a.Dir
	spec.Stdin = prompt
	spec.LogPath = logPath
	spec.Stream = a.Stream

	// The parser reads every line the runner writes to the log, while the
	// agent runs, and reports the events it finds. Flush reports what is
	// still open when the run ends.
	var p lineParser = activity.NewCodex(logPath, events)
	if choice.CLI.ID == agents.ClaudeID {
		p = activity.NewClaude(logPath, events)
	}
	spec.OnLine = p.Line
	defer p.Flush()

	res, err := runner.Run(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	read := agents.Read(choice.CLI, res, lastMessage)
	out := Result{
		ExitCode:     res.ExitCode,
		Canceled:     res.Canceled,
		SessionID:    read.SessionID,
		FinalMessage: read.Message,
	}

	// The full log is the most complete output; fall back to what the
	// runner kept in memory if it cannot be read. The log's time stamps are
	// removed so the usage-limit checks see the raw lines.
	full := res.Head + "\n" + res.Tail
	if data, err := os.ReadFile(logPath); err == nil {
		full = runner.StripStamps(string(data))
	}
	out.Output = full + "\n" + out.FinalMessage
	if choice.CLI.ID == agents.ClaudeID {
		// The envelope goes last, where ClaudeUsageLimit looks for it.
		out.Output += "\n" + strings.TrimSpace(res.Stdout)
	}
	return out, nil
}
