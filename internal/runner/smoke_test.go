package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSmokeRealAgents runs the real codex and claude once each with a trivial
// prompt, and codex once more running `go test ./...` inside its sandbox. It
// uses subscription quota, so it only runs when HUDDLE_SMOKE is set
// to the path of a clone of the target repo:
//
//	HUDDLE_SMOKE=~/projects/huddle-sandbox go test -run Smoke -v ./internal/runner
func TestSmokeRealAgents(t *testing.T) {
	dir := os.Getenv("HUDDLE_SMOKE")
	if dir == "" {
		t.Skip("set HUDDLE_SMOKE to a target repo clone to run the real agent CLIs")
	}
	const prompt = "Reply with the word OK"
	const goTestPrompt = "Run `go test ./...` in this repository. If it passes, reply with only the word OK. " +
		"If it fails, reply with FAIL followed by the error."
	logDir := t.TempDir()
	caches, err := GoCacheDirs(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name string
		spec Spec
	}{
		{"codex", Dev(dir, prompt, caches)},
		{"codex go test", Dev(dir, goTestPrompt, caches)},
		{"claude", PM(dir, prompt)},
	} {
		spec := tt.spec
		t.Run(tt.name, func(t *testing.T) {
			spec.Timeout = 5 * time.Minute
			spec.Stream = os.Stdout
			spec.LogPath = filepath.Join(logDir, strings.ReplaceAll(tt.name, " ", "-")+".log")
			res, err := Run(context.Background(), spec)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			t.Logf("exit=%d timed_out=%t duration=%s", res.ExitCode, res.TimedOut, res.Duration.Round(time.Second))
			if res.ExitCode != 0 || res.TimedOut {
				t.Fatalf("agent failed; tail:\n%s", res.Tail)
			}
			answer := res.Stdout
			if spec.Name == "claude" {
				var env struct {
					Type    string `json:"type"`
					IsError bool   `json:"is_error"`
					Result  string `json:"result"`
				}
				if err := json.Unmarshal([]byte(res.Stdout), &env); err != nil {
					t.Fatalf("claude stdout is not a JSON envelope: %v\n%s", err, res.Stdout)
				}
				if env.IsError {
					t.Fatalf("claude envelope reports an error: %s", res.Stdout)
				}
				answer = env.Result
				t.Logf("envelope type=%q result=%q", env.Type, env.Result)
			}
			if !strings.Contains(answer, "OK") || strings.Contains(answer, "FAIL") {
				t.Errorf("answer does not contain OK:\n%s", answer)
			}
		})
	}
}
