package agents

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// TestSmokeRealAgents runs each real CLI fresh, asking it to remember a word,
// then resumes the same session and asks for the word back. It uses
// subscription quota, so it only runs when HUDDLE_SMOKE is set to the path of
// a clone of the target repo:
//
//	HUDDLE_SMOKE=~/projects/huddle-sandbox go test -count=1 -run Smoke -v ./internal/agents
func TestSmokeRealAgents(t *testing.T) {
	dir := os.Getenv("HUDDLE_SMOKE")
	if dir == "" {
		t.Skip("set HUDDLE_SMOKE to a target repo clone to run the real agent CLIs")
	}
	const (
		word     = "PELICAN"
		first    = "Remember the word " + word + ". Reply with only the word OK. Do not run any commands or edit any files."
		followUp = "What word did I ask you to remember? Reply with only that word. Do not run any commands or edit any files."
	)
	logDir := t.TempDir()

	// run runs one turn of c as the Dev and reads it the way Huddle does.
	run := func(t *testing.T, name string, c Choice, prompt, sessionID string) Output {
		t.Helper()
		last := filepath.Join(logDir, name+".last")
		spec := Command(session.Dev, c, sessionID, last)
		spec.Dir, spec.Stdin = dir, prompt
		spec.Stream = os.Stdout
		spec.LogPath = filepath.Join(logDir, name+".log")
		res, err := runner.Run(context.Background(), spec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		t.Logf("%s: exit=%d duration=%s", name, res.ExitCode, res.Duration.Round(time.Second))
		out := Read(c.CLI, res, last)
		if out.Failed {
			t.Fatalf("%s failed; tail:\n%s", name, res.Tail)
		}
		return out
	}

	for _, cli := range All() {
		t.Run(cli.ID, func(t *testing.T) {
			c := Choice{CLI: cli}
			out := run(t, cli.ID+"-fresh", c, first, "")
			if out.SessionID == "" || !strings.Contains(out.Message, "OK") {
				t.Fatalf("fresh run: session id %q, final message %q", out.SessionID, out.Message)
			}
			t.Logf("model used: %s", out.Model)
			again := run(t, cli.ID+"-resume", c, followUp, out.SessionID)
			if again.SessionID != out.SessionID || !strings.Contains(strings.ToUpper(again.Message), word) {
				t.Errorf("resume: session id %q (want %q), final message %q (want %s)", again.SessionID, out.SessionID, again.Message, word)
			}
		})
	}
}

// TestSmokeValidate runs Validate against the real CLIs: each CLI's
// default model and a known model with an effort must pass, and a made-up
// model must fail with a readable error. Run it like TestSmokeRealAgents.
func TestSmokeValidate(t *testing.T) {
	if os.Getenv("HUDDLE_SMOKE") == "" {
		t.Skip("set HUDDLE_SMOKE to run the real agent CLIs")
	}
	ctx := context.Background()
	for _, cli := range All() {
		t.Run(cli.ID, func(t *testing.T) {
			if err := Validate(ctx, cli, "", ""); err != nil {
				t.Errorf("Validate(defaults) = %v", err)
			}
			known := cli.KnownModels[0].Value
			if err := Validate(ctx, cli, known, "low"); err != nil {
				t.Errorf("Validate(%s, low) = %v", known, err)
			}
			err := Validate(ctx, cli, "no-such-model-x", "")
			if err == nil || !strings.Contains(err.Error(), `rejected model "no-such-model-x"`) {
				t.Errorf("Validate(unknown model) = %v, want a rejected model error", err)
			}
			t.Logf("unknown model: %v", err)
		})
	}
}
