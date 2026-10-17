package companion

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// fakeRuns plays the companion's CLI: each call takes the next result.
// If play is set, each call runs the next function instead, which can
// act like the CLI (write its last-message file, say).
type fakeRuns struct {
	results []runner.Result
	play    []func(runner.Spec) runner.Result
	specs   []runner.Spec
}

func (f *fakeRuns) run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	f.specs = append(f.specs, spec)
	if len(f.play) > 0 {
		next := f.play[0]
		f.play = f.play[1:]
		return next(spec), nil
	}
	res := f.results[0]
	f.results = f.results[1:]
	return res, nil
}

// envelope returns Claude Code's JSON envelope as stdout.
func envelope(sessionID, result string, isError bool) runner.Result {
	data, _ := json.Marshal(agents.ClaudeEnvelope{Type: "result", Subtype: "success", IsError: isError, SessionID: sessionID, Result: result})
	code := 0
	if isError {
		code = 1
	}
	return runner.Result{ExitCode: code, Stdout: string(data)}
}

func newAgent(t *testing.T, runs *fakeRuns) Agent {
	home := t.TempDir()
	return Agent{
		Choice:    agents.Choice{CLI: agents.Claude()},
		Dir:       "/repo",
		Home:      home,
		Milestone: "Milestone 3",
		Sessions:  session.NewCompanion(home, "o/r", 58),
		Run:       runs.run,
		Now:       func() time.Time { return time.Date(2026, 10, 14, 0, 2, 0, 0, time.UTC) },
	}
}

func TestAskStartsThenResumesASession(t *testing.T) {
	runs := &fakeRuns{results: []runner.Result{envelope("s1", "First.", false), envelope("s1", "Second.", false)}}
	a := newAgent(t, runs)

	reply, err := a.Ask(context.Background(), "HUDDLE CONTEXT ...", "Why JSON?")
	if err != nil || reply != "First." {
		t.Fatalf("Ask() = %q, %v", reply, err)
	}
	first := runs.specs[0]
	if !strings.HasPrefix(first.Stdin, "YOU ARE HUDDLE'S COMPANION") || !strings.Contains(first.Stdin, `milestone "Milestone 3"`) {
		t.Errorf("the first message must start with the instructions:\n%s", first.Stdin)
	}
	if !strings.HasSuffix(first.Stdin, "HUDDLE CONTEXT ...\n\nTHE HUMAN SAYS:\nWhy JSON?\n") {
		t.Errorf("the message must end with the context and the human's words:\n%s", first.Stdin)
	}
	if strings.Contains(strings.Join(first.Args, " "), "--resume") {
		t.Error("the first message must start a fresh session")
	}
	if !strings.Contains(first.LogPath, "logs/companion/20261014-000200.000-companion.log") {
		t.Errorf("LogPath = %q", first.LogPath)
	}

	if _, err := a.Ask(context.Background(), "HUDDLE CONTEXT ...", "Later?"); err != nil {
		t.Fatal(err)
	}
	second := runs.specs[1]
	if !strings.Contains(strings.Join(second.Args, " "), "--resume s1") {
		t.Errorf("the second message must resume s1: %v", second.Args)
	}
	if strings.Contains(second.Stdin, "YOU ARE HUDDLE'S COMPANION") {
		t.Error("a resumed message must not repeat the instructions")
	}
}

func TestAskStartsFreshWhenTheSessionIsGone(t *testing.T) {
	gone := runner.Result{ExitCode: 1, Tail: "No conversation found with session ID: old"}
	runs := &fakeRuns{results: []runner.Result{gone, envelope("s2", "Hello again.", false)}}
	a := newAgent(t, runs)
	if err := a.Sessions.Save(session.Companion, "old"); err != nil {
		t.Fatal(err)
	}

	reply, err := a.Ask(context.Background(), "ctx", "hi")
	if err != nil || reply != "Hello again." {
		t.Fatalf("Ask() = %q, %v", reply, err)
	}
	if len(runs.specs) != 2 || !strings.HasPrefix(runs.specs[1].Stdin, "YOU ARE HUDDLE'S COMPANION") {
		t.Error("want a second, fresh run with the instructions")
	}
	if got := a.Sessions.Load().Get(session.Companion); got != "s2" {
		t.Errorf("saved session = %q, want s2", got)
	}
}

func TestAskKeepsTheSessionOnAnErrorReply(t *testing.T) {
	runs := &fakeRuns{results: []runner.Result{envelope("s1", "You've hit your limit · resets 3am", true)}}
	a := newAgent(t, runs)
	if err := a.Sessions.Save(session.Companion, "s1"); err != nil {
		t.Fatal(err)
	}
	_, err := a.Ask(context.Background(), "ctx", "hi")
	if err == nil || !strings.Contains(err.Error(), "You've hit your limit") {
		t.Errorf("Ask() error = %v, want the CLI's message", err)
	}
	if len(runs.specs) != 1 {
		t.Errorf("%d runs, want 1: an error reply is not a lost session", len(runs.specs))
	}
	if got := a.Sessions.Load().Get(session.Companion); got != "s1" {
		t.Errorf("saved session = %q, want s1 kept", got)
	}
}

// codexRun plays one codex exec run: the header with the session id, and
// the final message written to the --output-last-message file.
func codexRun(t *testing.T, sessionID, message string) func(runner.Spec) runner.Result {
	return func(spec runner.Spec) runner.Result {
		i := slices.Index(spec.Args, "--output-last-message")
		if i < 0 {
			t.Fatalf("no --output-last-message in %v", spec.Args)
		}
		// runner.Run would have created the log folder.
		if err := os.MkdirAll(filepath.Dir(spec.Args[i+1]), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(spec.Args[i+1], []byte(message), 0o644); err != nil {
			t.Fatal(err)
		}
		return runner.Result{Head: "OpenAI Codex v0.160.0\n--------\nsession id: " + sessionID + "\n--------"}
	}
}

func TestAskOnCodex(t *testing.T) {
	runs := &fakeRuns{}
	a := newAgent(t, runs)
	a.Home = t.TempDir() // the last-message file is written under Home
	a.Choice = agents.Choice{CLI: agents.Codex(), Model: "gpt-6.1-sol", Effort: "low"}
	first, second := codexRun(t, "cx-1", "PR #73 is waiting for CI."), codexRun(t, "cx-1", "Still waiting.")
	runs.play = []func(runner.Spec) runner.Result{first, second}

	reply, err := a.Ask(context.Background(), "ctx", "What is Codex doing?")
	if err != nil || reply != "PR #73 is waiting for CI." {
		t.Fatalf("Ask() = %q, %v", reply, err)
	}
	args := strings.Join(runs.specs[0].Args, " ")
	for _, want := range []string{"exec --strict-config", `sandbox_mode="read-only"`, "-m gpt-6.1-sol", `model_reasoning_effort="low"`} {
		if !strings.Contains(args, want) {
			t.Errorf("Codex companion command lacks %q: %s", want, args)
		}
	}
	if got := a.Sessions.Load().Get(session.Companion); got != "cx-1" {
		t.Errorf("saved session = %q, want cx-1", got)
	}

	if reply, err := a.Ask(context.Background(), "ctx", "And now?"); err != nil || reply != "Still waiting." {
		t.Fatalf("second Ask() = %q, %v", reply, err)
	}
	if args := runs.specs[1].Args; !slices.Contains(args, "resume") || args[len(args)-2] != "cx-1" {
		t.Errorf("the second message must resume cx-1: %v", args)
	}
}

func TestInstructionsInPlanMode(t *testing.T) {
	running, planning := Instructions("Milestone 3"), Instructions("")
	if !strings.Contains(running, `work on milestone "Milestone 3"`) {
		t.Errorf("running instructions do not name the milestone:\n%s", running)
	}
	if strings.Contains(planning, "{{milestone}}") || strings.Contains(planning, `milestone ""`) ||
		!strings.Contains(planning, "no milestone is running yet") {
		t.Errorf("planning instructions:\n%s", planning)
	}
	if !strings.Contains(planning, "<plan>") || !strings.Contains(running, "<plan>") {
		t.Error("the plan format is missing")
	}
}
