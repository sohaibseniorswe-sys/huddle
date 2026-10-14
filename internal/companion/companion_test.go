package companion

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// fakeRuns plays the companion's CLI: each call takes the next result.
type fakeRuns struct {
	results []runner.Result
	specs   []runner.Spec
}

func (f *fakeRuns) run(_ context.Context, spec runner.Spec) (runner.Result, error) {
	f.specs = append(f.specs, spec)
	res := f.results[0]
	f.results = f.results[1:]
	return res, nil
}

// envelope returns Claude Code's JSON envelope as stdout.
func envelope(sessionID, result string, isError bool) runner.Result {
	data, _ := json.Marshal(runner.ClaudeEnvelope{Type: "result", Subtype: "success", IsError: isError, SessionID: sessionID, Result: result})
	code := 0
	if isError {
		code = 1
	}
	return runner.Result{ExitCode: code, Stdout: string(data)}
}

func newAgent(t *testing.T, runs *fakeRuns) Agent {
	home := t.TempDir()
	return Agent{
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
	if !strings.Contains(first.LogPath, ".huddle/logs/companion/20261014-000200.000-companion.log") {
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
