package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSmokeRealAgents runs each real CLI fresh, asking it to remember a word,
// then resumes the same session and asks for the word back. It uses
// subscription quota, so it only runs when HUDDLE_SMOKE is set to the path of
// a clone of the target repo:
//
//	HUDDLE_SMOKE=~/projects/huddle-sandbox go test -count=1 -run Smoke -v ./internal/runner
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

	// run runs spec through the runner and returns its session id and final
	// message, extracted the way Huddle does.
	run := func(t *testing.T, name string, spec Spec, lastMsg string) (string, string) {
		t.Helper()
		spec.Stream = os.Stdout
		spec.LogPath = filepath.Join(logDir, name+".log")
		res, err := Run(context.Background(), spec)
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		t.Logf("%s: exit=%d duration=%s", name, res.ExitCode, res.Duration.Round(time.Second))
		if res.ExitCode != 0 {
			t.Fatalf("%s failed; tail:\n%s", name, res.Tail)
		}
		if spec.Name == "codex" {
			return CodexSessionID(res.Head), CodexFinalMessage(lastMsg, res)
		}
		env, err := ParseClaude(res.Stdout)
		if err != nil {
			t.Fatal(err)
		}
		if env.IsError {
			t.Fatalf("claude envelope reports an error: %s", res.Stdout)
		}
		return env.SessionID, ClaudeFinalMessage(res)
	}

	t.Run("codex", func(t *testing.T) {
		last := filepath.Join(logDir, "codex-last.txt")
		id, msg := run(t, "codex-fresh", Dev(dir, first, "", last), last)
		if id == "" || !strings.Contains(msg, "OK") {
			t.Fatalf("fresh run: session id %q, final message %q", id, msg)
		}
		id2, msg := run(t, "codex-resume", Dev(dir, followUp, id, last), last)
		if id2 != id || !strings.Contains(strings.ToUpper(msg), word) {
			t.Errorf("resume: session id %q (want %q), final message %q (want %s)", id2, id, msg, word)
		}
	})

	t.Run("claude", func(t *testing.T) {
		id, msg := run(t, "claude-fresh", PM(dir, first, ""), "")
		if id == "" || !strings.Contains(msg, "OK") {
			t.Fatalf("fresh run: session id %q, final message %q", id, msg)
		}
		id2, msg := run(t, "claude-resume", PM(dir, followUp, id), "")
		if id2 != id || !strings.Contains(strings.ToUpper(msg), word) {
			t.Errorf("resume: session id %q (want %q), final message %q (want %s)", id2, id, msg, word)
		}
	})
}
