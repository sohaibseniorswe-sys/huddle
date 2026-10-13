package loop

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

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

// CLIAgents runs the real agent CLIs (section 6) in the repo root, streaming
// their output to Stream and logging it under Home/.huddle/logs.
type CLIAgents struct {
	Dir    string    // repo root
	Home   string    // home directory, for ~/.huddle/logs
	Stream io.Writer // live output, normally os.Stdout
}

func (a CLIAgents) Run(ctx context.Context, role session.Role, sessionID, prompt string, turn int) (Result, error) {
	logPath := runner.LogPath(a.Home, time.Now(), turn, string(role))
	lastMessage := strings.TrimSuffix(logPath, ".log") + ".last"
	var spec runner.Spec
	if role == session.PM {
		spec = runner.PM(a.Dir, prompt, sessionID)
	} else {
		spec = runner.Dev(a.Dir, prompt, sessionID, lastMessage)
	}
	spec.LogPath = logPath
	spec.Stream = a.Stream

	res, err := runner.Run(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	out := Result{ExitCode: res.ExitCode, Canceled: res.Canceled}

	// The full log is the most complete output; fall back to what the
	// runner kept in memory if it cannot be read.
	full := res.Head + "\n" + res.Tail
	if data, err := os.ReadFile(logPath); err == nil {
		full = string(data)
	}
	if role == session.PM {
		if env, err := runner.ParseClaude(res.Stdout); err == nil {
			out.SessionID = env.SessionID
		}
		out.FinalMessage = runner.ClaudeFinalMessage(res)
		// The envelope goes last, where ClaudeUsageLimit looks for it.
		out.Output = full + "\n" + out.FinalMessage + "\n" + strings.TrimSpace(res.Stdout)
	} else {
		out.SessionID = runner.CodexSessionID(res.Head)
		out.FinalMessage = runner.CodexFinalMessage(lastMessage, res)
		out.Output = full + "\n" + out.FinalMessage
	}
	return out, nil
}
