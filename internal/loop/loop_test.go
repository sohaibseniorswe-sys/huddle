package loop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

const (
	testRepo  = "sohaibseniorswe-sys/huddle-sandbox"
	controlNo = 3
)

// fakeClock advances only when slept on. onSleep, if set, runs after each
// sleep (with the number of sleeps so far) and may change the fake GitHub
// state or cancel the run.
type fakeClock struct {
	t       *testing.T
	now     time.Time
	sleeps  []time.Duration
	onSleep func(n int)
	cancel  context.CancelFunc
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
	if c.onSleep != nil {
		c.onSleep(len(c.sleeps))
	}
	if len(c.sleeps) > 100 {
		c.t.Error("loop did not stop after 100 sleeps")
		c.cancel()
	}
	return ctx.Err()
}

// call is one recorded agent run.
type call struct {
	role      session.Role
	sessionID string
	prompt    string
}

// fakeAgents plays back steps, one per run. A step may change the fake
// GitHub state, as a real agent would with gh.
type fakeAgents struct {
	t     *testing.T
	steps []func(ctx context.Context, c call) Result
	calls []call
}

func (a *fakeAgents) Run(ctx context.Context, role session.Role, sessionID, prompt string, _ int, _ func(activity.Event)) (Result, error) {
	c := call{role, sessionID, prompt}
	a.calls = append(a.calls, c)
	if len(a.calls) > len(a.steps) {
		a.t.Fatalf("unexpected run %d: %+v", len(a.calls), c)
	}
	return a.steps[len(a.calls)-1](ctx, c), nil
}

type env struct {
	t        *testing.T
	ctx      context.Context
	gh       *gh.Fake
	clock    *fakeClock
	agents   *fakeAgents
	sessions session.Store
	out      *bytes.Buffer
	loop     *Loop
}

func newEnv(t *testing.T, labels ...string) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	e := &env{
		t:   t,
		ctx: ctx,
		gh: &gh.Fake{Issues: []gh.Issue{{
			Number: controlNo, Title: "Sandbox Control", Milestone: "Sandbox M1",
			Labels: append([]string{"control"}, labels...),
		}}},
		clock:    &fakeClock{t: t, now: time.Date(2026, 10, 12, 10, 0, 0, 0, time.FixedZone("UTC+1", 3600)), cancel: cancel},
		agents:   &fakeAgents{t: t},
		sessions: session.New(t.TempDir(), testRepo, controlNo),
		out:      &bytes.Buffer{},
	}
	e.loop = &Loop{
		Control:   gh.Control{Client: e.gh, Repo: testRepo, Number: controlNo},
		Milestone: "Sandbox M1",
		Agents:    e.agents,
		Team:      defaultTeam(),
		Sessions:  e.sessions,
		Clock:     e.clock,
		Report:    &recorder{plain: PlainReporter{Out: e.out}},
		Controls:  &Controls{},
	}
	return e
}

// defaultTeam is Huddle's default: Codex as the Dev, Claude Code as the PM
// and the companion.
func defaultTeam() agents.Team {
	return agents.Team{
		Dev:       agents.Choice{CLI: agents.Codex()},
		PM:        agents.Choice{CLI: agents.Claude()},
		Companion: agents.Choice{CLI: agents.Claude()},
	}
}

// swappedTeam has Claude Code as the Dev and Codex as the PM.
func swappedTeam() agents.Team {
	return agents.Team{
		Dev:       agents.Choice{CLI: agents.Claude(), Model: "sonnet", Effort: "high"},
		PM:        agents.Choice{CLI: agents.Codex(), Model: "gpt-6.1-sol", Effort: "xhigh"},
		Companion: agents.Choice{CLI: agents.Codex()},
	}
}

// recorder is a Reporter that prints like --plain and also keeps every
// status it is sent.
type recorder struct {
	plain    PlainReporter
	statuses []Status
}

func (r *recorder) Event(ev activity.Event) { r.plain.Event(ev) }
func (r *recorder) Status(s Status)         { r.statuses = append(r.statuses, s) }

// phases returns the phases reported so far.
func (e *env) phases() []Phase {
	var out []Phase
	for _, s := range e.loop.Report.(*recorder).statuses {
		out = append(out, s.Phase)
	}
	return out
}

func (e *env) run() int {
	e.t.Helper()
	code := e.loop.Run(e.ctx)
	e.t.Logf("output:\n%s", e.out)
	return code
}

// labels returns the control issue's state labels.
func (e *env) states() []string {
	var out []string
	for _, l := range e.gh.Issues[0].Labels {
		if strings.HasPrefix(l, gh.StatePrefix) {
			out = append(out, l)
		}
	}
	return out
}

func (e *env) wantStates(want ...string) {
	e.t.Helper()
	if got := e.states(); !slices.Equal(got, want) {
		e.t.Errorf("state labels = %q, want %q", got, want)
	}
}

func (e *env) comments() []string {
	var out []string
	for _, c := range e.gh.IssueComments[controlNo] {
		out = append(out, c.Body)
	}
	return out
}

// agentSets is what an agent does with gh issue edit --add-label: it adds a
// label and leaves Huddle's *-working label in place.
func (e *env) agentSets(label string) {
	e.gh.Issues[0].Labels = append(e.gh.Issues[0].Labels, label)
}

// humanSets is the human switching the label in the GitHub UI.
func (e *env) humanSets(label string) {
	e.gh.Issues[0].Labels = []string{"control", label}
}

func (e *env) agentComments(body string) {
	if e.gh.IssueComments == nil {
		e.gh.IssueComments = map[int][]gh.Comment{}
	}
	e.gh.IssueComments[controlNo] = append(e.gh.IssueComments[controlNo], gh.Comment{Author: "agent", Body: body, URL: fmt.Sprintf("https://github.com/%s/issues/%d#c%d", testRepo, controlNo, len(e.gh.IssueComments[controlNo])+1)})
}

// sets returns a step that sets label, as an agent ending its turn.
func (e *env) sets(label, sessionID string) func(context.Context, call) Result {
	return func(context.Context, call) Result {
		e.agentSets(label)
		return Result{SessionID: sessionID}
	}
}

func countMarker(comments []string, marker string) int {
	n := 0
	for _, c := range comments {
		if strings.Contains(c, marker) {
			n++
		}
	}
	return n
}

func TestDevThenPMThenDone(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			if got := e.states(); !slices.Equal(got, []string{gh.StateDevWorking}) {
				t.Errorf("labels during the Dev run = %q", got)
			}
			e.agentComments("Status: Completed")
			e.agentSets(gh.StatePMReady)
			return Result{SessionID: "dev-1"}
		},
		func(context.Context, call) Result {
			if got := e.states(); !slices.Equal(got, []string{gh.StatePMWorking}) {
				t.Errorf("labels during the PM run = %q", got)
			}
			e.agentComments("FINAL REPORT: all done")
			e.agentSets(gh.StateDone)
			return Result{SessionID: "pm-1"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d, want 0", code)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[0].role != session.Dev || e.agents.calls[1].role != session.PM {
		t.Fatalf("calls = %+v", e.agents.calls)
	}
	for i, c := range e.agents.calls {
		if c.sessionID != "" {
			t.Errorf("call %d resumed %q, want a fresh session", i, c.sessionID)
		}
		if !strings.Contains(c.prompt, "It's your turn. Read control issue #3 and continue.") || !strings.Contains(c.prompt, `milestone "Sandbox M1"`) {
			t.Errorf("call %d prompt:\n%s", i, c.prompt)
		}
	}
	if !strings.Contains(e.agents.calls[0].prompt, "YOUR ROLE: DEV") || !strings.Contains(e.agents.calls[1].prompt, "YOUR ROLE: PM") {
		t.Error("role sections are wrong")
	}
	if len(e.clock.sleeps) != 0 {
		t.Errorf("slept %v; the label must be read again straight after a run", e.clock.sleeps)
	}
	if !strings.Contains(e.out.String(), "FINAL REPORT: all done") {
		t.Error("final report not printed")
	}
	if got := e.sessions.Load(); got != (session.IDs{Dev: "dev-1", PM: "pm-1"}) {
		t.Errorf("sessions = %+v", got)
	}
	e.wantStates(gh.StateDone)
	if n := len(e.comments()); n != 2 {
		t.Errorf("%d comments, want only the agents' 2: %q", n, e.comments())
	}
}

func TestAntiStall(t *testing.T) {
	tests := []struct {
		name     string
		start    string
		exitCode int
		wantRole session.Role
		next     string
	}{
		{"dev", gh.StateDevReady, 0, session.Dev, gh.StatePMReady},
		{"pm", gh.StatePMReady, 0, session.PM, gh.StateDevReady},
		{"dev non-zero exit", gh.StateDevReady, 3, session.Dev, gh.StatePMReady},
		{"pm non-zero exit", gh.StatePMReady, 1, session.PM, gh.StateDevReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.start)
			e.loop.Once = true
			e.agents.steps = []func(context.Context, call) Result{
				func(context.Context, call) Result {
					return Result{ExitCode: tt.exitCode, SessionID: "s1", FinalMessage: "I stopped halfway; `go test` fails.", Output: "lots of output"}
				},
			}
			if code := e.run(); code != ExitOK {
				t.Fatalf("exit %d", code)
			}
			if e.agents.calls[0].role != tt.wantRole {
				t.Errorf("ran %s", e.agents.calls[0].role)
			}
			e.wantStates(tt.next)
			c := e.comments()
			if len(c) != 1 {
				t.Fatalf("comments = %q, want one handoff", c)
			}
			for _, want := range []string{
				"<!-- huddle:auto-handoff role=" + string(tt.wantRole) + " -->",
				"I stopped halfway; `go test` fails.",
				"exit code " + string(rune('0'+tt.exitCode)),
				tt.next,
			} {
				if !strings.Contains(c[0], want) {
					t.Errorf("handoff comment missing %q:\n%s", want, c[0])
				}
			}
			if got := e.sessions.Load().Get(tt.wantRole); got != "s1" {
				t.Errorf("session = %q, want kept", got)
			}
		})
	}
}

func TestResumeFailsThenFreshRun(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	if err := e.sessions.Save(session.Dev, "old"); err != nil {
		t.Fatal(err)
	}
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			if got := e.states(); !slices.Equal(got, []string{gh.StateDevWorking}) {
				t.Errorf("labels = %q", got)
			}
			return Result{ExitCode: 1, FinalMessage: "Error: thread/resume failed: no rollout found"}
		},
		e.sets(gh.StatePMReady, "new"),
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[0].sessionID != "old" || e.agents.calls[1].sessionID != "" {
		t.Fatalf("calls = %+v, want a resume of old then a fresh run", e.agents.calls)
	}
	if got := e.sessions.Load().Dev; got != "new" {
		t.Errorf("dev session = %q, want new", got)
	}
	if c := e.comments(); len(c) != 0 {
		t.Errorf("comments = %q, want no handoff", c)
	}
	e.wantStates(gh.StatePMReady)
}

func TestResumeFailsAndFreshRunStalls(t *testing.T) {
	e := newEnv(t, gh.StatePMReady)
	e.loop.Once = true
	e.sessions.Save(session.PM, "old")
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result { return Result{ExitCode: 1, FinalMessage: "No conversation found"} },
		func(context.Context, call) Result {
			return Result{ExitCode: 1, SessionID: "new", FinalMessage: "fresh also failed"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 2 {
		t.Fatalf("calls = %+v, want exactly one fallback", e.agents.calls)
	}
	c := e.comments()
	if len(c) != 1 || !strings.Contains(c[0], "fresh also failed") || !strings.Contains(c[0], "role=pm") {
		t.Errorf("comments = %q, want one handoff with the fresh run's output", c)
	}
	e.wantStates(gh.StateDevReady)
	if got := e.sessions.Load().PM; got != "new" {
		t.Errorf("pm session = %q", got)
	}
}

func TestUsageLimitWithResetTime(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	limited := "OpenAI Codex v0.160.0\nsession id: dev-1\nERROR: You’ve hit your usage limit. Try again at 3:45 PM.\n"
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result { return Result{ExitCode: 1, SessionID: "dev-1", Output: limited} },
		e.sets(gh.StatePMReady, "dev-1"),
	}
	e.clock.onSleep = func(int) {
		if got := e.states(); !slices.Equal(got, []string{gh.StateDevWorking}) {
			t.Errorf("labels during the wait = %q, want the working label kept", got)
		}
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	// Reset at 15:45, plus a minute, from 10:00.
	if want := []time.Duration{5*time.Hour + 46*time.Minute}; !slices.Equal(e.clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", e.clock.sleeps, want)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[1].role != session.Dev || e.agents.calls[1].sessionID != "dev-1" {
		t.Fatalf("calls = %+v, want the same Dev session resumed", e.agents.calls)
	}
	c := e.comments()
	if len(c) != 1 || !strings.Contains(c[0], "<!-- huddle:usage-wait role=dev -->") || !strings.Contains(c[0], "2026-10-12 15:45") {
		t.Errorf("comments = %q, want one usage-wait comment with the reset time", c)
	}
	if countMarker(c, "huddle:auto-handoff") != 0 {
		t.Error("handed over on a usage limit")
	}
	e.wantStates(gh.StatePMReady)
}

func TestUsageLimitWithoutResetTime(t *testing.T) {
	e := newEnv(t, gh.StatePMReady)
	e.loop.Once = true
	e.sessions.Save(session.PM, "pm-1")
	limited := func(context.Context, call) Result {
		return Result{ExitCode: 1, SessionID: "pm-1",
			Output: `{"type":"result","is_error":true,"session_id":"pm-1","result":"You've hit your usage limit · contact your admin to increase it"}`}
	}
	e.agents.steps = []func(context.Context, call) Result{limited, limited, limited, e.sets(gh.StateDevReady, "pm-1")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if want := []time.Duration{UsageRetry, UsageRetry, UsageRetry}; !slices.Equal(e.clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", e.clock.sleeps, want)
	}
	for i, c := range e.agents.calls {
		if c.role != session.PM || c.sessionID != "pm-1" {
			t.Errorf("call %d = %+v, want the PM session resumed (a limited run never falls back to a fresh one)", i, c)
		}
	}
	c := e.comments()
	if len(c) != 1 || countMarker(c, "<!-- huddle:usage-wait role=pm -->") != 1 || !strings.Contains(c[0], "every 15 minutes") {
		t.Errorf("comments = %q, want exactly one usage-wait comment", c)
	}
	e.wantStates(gh.StateDevReady)
}

func TestEscalate(t *testing.T) {
	tests := []struct {
		name   string
		start  string
		role   session.Role
		output string
		msg    string
	}{
		{"codex quota exceeded (API-key billing)", gh.StateDevReady, session.Dev,
			"session id: dev-1\nERROR: Quota exceeded. Check your plan and billing details.\n",
			"Quota exceeded. Check your plan and billing details."},
		{"codex out of credits", gh.StateDevReady, session.Dev,
			"ERROR: Your workspace is out of credits. Add credits to continue.\n",
			"Your workspace is out of credits. Add credits to continue."},
		{"claude seat type", gh.StatePMReady, session.PM,
			`{"type":"result","is_error":true,"session_id":"pm-1","result":"Your seat type doesn't include usage credits"}`,
			"Your seat type doesn't include usage credits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, tt.start)
			e.sessions.Save(tt.role, "s-1")
			e.agents.steps = []func(context.Context, call) Result{
				func(context.Context, call) Result { return Result{ExitCode: 1, Output: tt.output} },
			}
			// After escalating, the loop polls; stop it there.
			e.clock.onSleep = func(int) { e.clock.cancel() }
			if code := e.run(); code != ExitInterrupted {
				t.Fatalf("exit %d", code)
			}
			if len(e.agents.calls) != 1 {
				t.Errorf("calls = %+v, want no fresh-session fallback", e.agents.calls)
			}
			c := e.comments()
			if len(c) != 1 || !strings.Contains(c[0], "<!-- huddle:escalate role="+string(tt.role)+" -->") || !strings.Contains(c[0], tt.msg) {
				t.Fatalf("comments = %q, want one escalation with %q", c, tt.msg)
			}
			if countMarker(c, "huddle:auto-handoff") != 0 {
				t.Error("handed over on an escalation")
			}
			e.wantStates(gh.StateHumanRequired)
			if got := e.sessions.Load().Get(tt.role); got != "s-1" {
				t.Errorf("session = %q, want kept", got)
			}
			// The escalation comment is then printed as the human-required comment.
			if !strings.Contains(e.out.String(), "waiting for the human") {
				t.Error("human-required comment not printed")
			}
		})
	}
}

func TestEscalateOnRetryAfterWait(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			return Result{ExitCode: 1, SessionID: "d", Output: "ERROR: You’ve hit your usage limit. Try again later.\n"}
		},
		func(context.Context, call) Result {
			return Result{ExitCode: 1, SessionID: "d", Output: "ERROR: You hit your spend cap set in your workspace. Increase your spend cap to continue.\n"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	c := e.comments()
	if len(c) != 2 || countMarker(c, "huddle:usage-wait") != 1 || countMarker(c, "huddle:escalate") != 1 {
		t.Errorf("comments = %q, want a wait then an escalation", c)
	}
	e.wantStates(gh.StateHumanRequired)
}

func TestCtrlCDuringUsageWait(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			return Result{ExitCode: 1, SessionID: "dev-1", Output: "ERROR: You’ve hit your usage limit. Try again later.\n"}
		},
	}
	e.clock.onSleep = func(int) { e.clock.cancel() }
	if code := e.run(); code != ExitInterrupted {
		t.Fatalf("exit %d, want 130", code)
	}
	e.wantStates(gh.StateDevReady)
	if c := e.comments(); len(c) != 1 || countMarker(c, "huddle:usage-wait") != 1 {
		t.Errorf("comments = %q", c)
	}
	if got := e.sessions.Load().Dev; got != "dev-1" {
		t.Errorf("dev session = %q, want kept for the next huddle", got)
	}
}

func TestHumanRequired(t *testing.T) {
	e := newEnv(t, gh.StateHumanRequired)
	e.loop.Once = true
	e.agentComments("PM: I need the deploy key")
	e.clock.onSleep = func(n int) {
		if n == 3 {
			e.agentComments("Human: added the key, carry on")
			e.humanSets(gh.StateDevReady)
		}
	}
	e.agents.steps = []func(context.Context, call) Result{e.sets(gh.StatePMReady, "d")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if want := []time.Duration{PollInterval, PollInterval, PollInterval}; !slices.Equal(e.clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", e.clock.sleeps, want)
	}
	if n := strings.Count(e.out.String(), "I need the deploy key"); n != 1 {
		t.Errorf("latest comment printed %d times, want once", n)
	}
	if len(e.agents.calls) != 1 || e.agents.calls[0].role != session.Dev {
		t.Errorf("calls = %+v, want the Dev to run after the ready label", e.agents.calls)
	}
}

func TestHumanRequiredNewCommentPrintedAgain(t *testing.T) {
	e := newEnv(t, gh.StateHumanRequired)
	e.agentComments("comment-one")
	e.clock.onSleep = func(n int) {
		switch n {
		case 2:
			e.agentComments("comment-two")
		case 4:
			e.clock.cancel()
		}
	}
	if code := e.run(); code != ExitInterrupted {
		t.Fatalf("exit %d", code)
	}
	out := e.out.String()
	if strings.Count(out, "comment-one") != 1 || strings.Count(out, "comment-two") != 1 {
		t.Errorf("want each comment printed once:\n%s", out)
	}
}

func TestInterruptedRunRecovery(t *testing.T) {
	for _, tt := range []struct {
		label string
		role  session.Role
	}{{gh.StateDevWorking, session.Dev}, {gh.StatePMWorking, session.PM}} {
		t.Run(tt.label, func(t *testing.T) {
			e := newEnv(t, tt.label)
			e.loop.Once = true
			e.sessions.Save(tt.role, "interrupted-session")
			e.agents.steps = []func(context.Context, call) Result{e.sets(readyLabel(other(tt.role)), "interrupted-session")}
			if code := e.run(); code != ExitOK {
				t.Fatalf("exit %d", code)
			}
			if c := e.agents.calls; len(c) != 1 || c[0].role != tt.role || c[0].sessionID != "interrupted-session" {
				t.Errorf("calls = %+v, want the interrupted session resumed", c)
			}
			e.wantStates(readyLabel(other(tt.role)))
		})
	}
}

func TestNoLabelPolls(t *testing.T) {
	e := newEnv(t)
	e.clock.onSleep = func(n int) {
		if n == 5 {
			e.clock.cancel()
		}
	}
	if code := e.run(); code != ExitInterrupted {
		t.Fatalf("exit %d", code)
	}
	if n := strings.Count(e.out.String(), "add state:dev-ready to start"); n != 1 {
		t.Errorf("hint printed %d times, want once:\n%s", n, e.out)
	}
	if len(e.clock.sleeps) != 5 || slices.ContainsFunc(e.clock.sleeps, func(d time.Duration) bool { return d != PollInterval }) {
		t.Errorf("sleeps = %v, want 30 s polls", e.clock.sleeps)
	}
	if len(e.agents.calls) != 0 {
		t.Error("an agent ran without a label")
	}
}

func TestNoLabelThenStart(t *testing.T) {
	e := newEnv(t)
	e.loop.Once = true
	e.clock.onSleep = func(n int) {
		if n == 2 {
			e.agentSets(gh.StateDevReady)
		}
	}
	e.agents.steps = []func(context.Context, call) Result{e.sets(gh.StatePMReady, "d")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 1 {
		t.Errorf("calls = %+v", e.agents.calls)
	}
}

func TestCtrlCDuringRun(t *testing.T) {
	e := newEnv(t, gh.StatePMReady)
	e.agents.steps = []func(context.Context, call) Result{
		func(ctx context.Context, _ call) Result {
			e.clock.cancel() // Ctrl+C; the runner kills the process group
			if ctx.Err() == nil {
				t.Error("run context not canceled")
			}
			return Result{ExitCode: -1, Canceled: true, SessionID: "pm-new"}
		},
	}
	if code := e.run(); code != ExitInterrupted {
		t.Fatalf("exit %d, want 130", code)
	}
	e.wantStates(gh.StatePMReady)
	if got := e.sessions.Load().PM; got != "pm-new" {
		t.Errorf("pm session = %q, want saved so the next huddle resumes it", got)
	}
	if c := e.comments(); len(c) != 0 {
		t.Errorf("comments = %q, want none", c)
	}
}

func TestOnceStopsAfterOneTurn(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	e.agents.steps = []func(context.Context, call) Result{e.sets(gh.StatePMReady, "d")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 1 {
		t.Errorf("calls = %+v, want one turn", e.agents.calls)
	}
	e.wantStates(gh.StatePMReady)
}

func TestDoneAtStartup(t *testing.T) {
	e := newEnv(t, gh.StateDone)
	e.agentComments("the report")
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(e.out.String(), "the report") {
		t.Error("report not printed")
	}
}

func TestGitHubErrorsAreRetried(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	e.gh.IssueErr = errors.New("HTTP 502")
	e.clock.onSleep = func(n int) {
		if n == 2 {
			e.gh.IssueErr = nil
		}
	}
	e.agents.steps = []func(context.Context, call) Result{e.sets(gh.StatePMReady, "d")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 1 || !strings.Contains(e.out.String(), "HTTP 502") {
		t.Errorf("calls = %+v, output:\n%s", e.agents.calls, e.out)
	}
}

func TestFence(t *testing.T) {
	if got := fence("a ``` b"); !strings.HasPrefix(got, "````\n") {
		t.Errorf("fence() = %q", got)
	}
}

func TestPauseLetsTheTurnFinishAndResumeStartsTheNext(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.agents.steps = []func(context.Context, call) Result{
		func(ctx context.Context, _ call) Result {
			e.loop.Controls.Pause() // /pause while the Dev works
			if ctx.Err() != nil {
				t.Error("/pause canceled the running agent")
			}
			e.agentSets(gh.StatePMReady)
			return Result{SessionID: "dev-1"}
		},
		e.sets(gh.StateDone, "pm-1"),
	}
	e.clock.onSleep = func(n int) {
		if len(e.agents.calls) != 1 {
			t.Errorf("sleep %d: %d agent runs while paused, want 1", n, len(e.agents.calls))
		}
		if n == 3 {
			e.loop.Controls.Resume()
		}
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[1].role != session.PM {
		t.Errorf("calls = %+v, want the PM to run after /resume", e.agents.calls)
	}
	if len(e.clock.sleeps) != 3 {
		t.Errorf("sleeps = %v, want 3 polls while paused", e.clock.sleeps)
	}
	out := e.out.String()
	if strings.Count(out, "Paused. The PM (Claude Code) is next") != 1 || !strings.Contains(out, "Resumed") {
		t.Errorf("pause and resume not reported once each:\n%s", out)
	}
	want := []Phase{PhaseWorking, PhasePaused, PhaseWorking, PhaseDone}
	if got := e.phases(); !slices.Equal(got, want) {
		t.Errorf("phases = %v, want %v", got, want)
	}
}

func TestStatusReportsUsageWait(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Once = true
	limited := "ERROR: You’ve hit your usage limit. Try again at 3:45 PM.\n"
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result { return Result{ExitCode: 1, SessionID: "d", Output: limited} },
		e.sets(gh.StatePMReady, "d"),
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	statuses := e.loop.Report.(*recorder).statuses
	if len(statuses) != 3 || statuses[1].Phase != PhaseUsageWait || statuses[1].Role != session.Dev {
		t.Fatalf("statuses = %+v, want working, usage wait, working", statuses)
	}
	if got := statuses[1].Until.Format("15:04"); got != "15:46" {
		t.Errorf("usage wait until %s, want 15:46 (reset plus a minute)", got)
	}
	if statuses[0].Turn != 1 {
		t.Errorf("turn = %d, want 1", statuses[0].Turn)
	}
}

func TestControlsWake(t *testing.T) {
	c := &Controls{}

	// A wake during a nap ends it.
	ctx, cancel := c.napContext(context.Background())
	if ctx.Err() != nil {
		t.Fatal("nap ended before any wake")
	}
	c.Wake()
	if ctx.Err() == nil {
		t.Error("Wake did not end the nap")
	}
	cancel()

	// A wake between naps is kept for the next nap, then used up.
	c.Wake()
	ctx, cancel = c.napContext(context.Background())
	if ctx.Err() == nil {
		t.Error("a wake between naps was lost")
	}
	cancel()
	ctx, cancel = c.napContext(context.Background())
	if ctx.Err() != nil {
		t.Error("one wake ended two naps")
	}
	cancel()

	// Resume wakes too, and clears the pause.
	c.Pause()
	ctx, cancel = c.napContext(context.Background())
	c.Resume()
	if ctx.Err() == nil || c.Paused() {
		t.Error("Resume did not wake the loop or did not clear the pause")
	}
	cancel()
}

// TestSwappedRoles runs a Dev turn and a PM turn with Claude Code as the
// Dev and Codex as the PM: the prompts, Huddle's lines and its comments
// name the tool that really plays each role, and a Codex usage message
// counts for the PM.
func TestSwappedRoles(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.loop.Team = swappedTeam()
	e.agents.steps = []func(context.Context, call) Result{
		// The Dev stops without a label: Huddle hands over to the PM.
		func(context.Context, call) Result {
			return Result{SessionID: "dev-1", FinalMessage: "Closed #1."}
		},
		// The PM (Codex) hits a usage limit, then finishes the milestone.
		func(context.Context, call) Result {
			return Result{ExitCode: 1, SessionID: "pm-1", Output: "ERROR: You’ve hit your usage limit. Try again later."}
		},
		func(context.Context, call) Result {
			e.agentSets(gh.StateDone)
			return Result{SessionID: "pm-1"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	calls := e.agents.calls
	if len(calls) != 3 || calls[0].role != session.Dev || calls[1].role != session.PM {
		t.Fatalf("calls = %+v", calls)
	}
	if !strings.Contains(calls[0].prompt, "YOUR ROLE: DEV\nYou are the Dev agent (Claude Code).") {
		t.Errorf("Dev prompt does not name Claude Code:\n%s", calls[0].prompt)
	}
	if !strings.Contains(calls[1].prompt, "YOUR ROLE: PM\nYou are the PM agent (Codex).") {
		t.Errorf("PM prompt does not name Codex:\n%s", calls[1].prompt)
	}
	out := e.out.String()
	for _, want := range []string{"turn 1: Dev (Claude Code), fresh session", "turn 2: PM (Codex), fresh session", "PM (Codex) hit a usage limit"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	c := e.comments()
	if len(c) != 2 || !strings.Contains(c[0], "the Dev (Claude Code) ended its turn") || !strings.Contains(c[0], "to the PM (Codex)") ||
		!strings.Contains(c[1], "the PM (Codex) hit a usage limit") {
		t.Errorf("comments = %q", c)
	}
}

// Huddle's definition of done, row by row: state:done; closed with
// nothing else open; closed while work is open (reopened, the run goes on).

func TestDoneByLabel(t *testing.T) {
	for _, closed := range []bool{false, true} {
		e := newEnv(t, gh.StatePMReady)
		e.gh.Milestone = []gh.IssueState{{Number: 1}} // still open: does not matter
		e.agents.steps = []func(context.Context, call) Result{
			func(context.Context, call) Result {
				e.agentComments("FINAL REPORT: all done")
				e.agentSets(gh.StateDone)
				e.gh.Issues[0].Closed = closed
				return Result{SessionID: "pm-1"}
			},
		}
		if code := e.run(); code != ExitOK {
			t.Fatalf("closed=%v: exit %d, want 0", closed, code)
		}
		if !strings.Contains(e.out.String(), "state:done: the milestone is finished") || !strings.Contains(e.out.String(), "FINAL REPORT") {
			t.Errorf("closed=%v: no done line with the report", closed)
		}
		if len(e.gh.Reopened) != 0 {
			t.Errorf("closed=%v: reopened %v", closed, e.gh.Reopened)
		}
	}
}

func TestDoneByClosingWithNothingOpen(t *testing.T) {
	e := newEnv(t, gh.StatePMReady)
	e.gh.Milestone = []gh.IssueState{
		{Number: 1, Closed: true},
		{Number: 2, Closed: true},
		{Number: controlNo, Labels: []string{"control"}}, // the control issue never counts
	}
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			e.agentComments("FINAL REPORT: all done")
			e.gh.Issues[0].Closed = true // closed, label left at pm-working
			return Result{SessionID: "pm-1"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d, want 0", code)
	}
	out := e.out.String()
	if !strings.Contains(out, "control issue #3 is closed and every other issue of the milestone is closed") || !strings.Contains(out, "FINAL REPORT") {
		t.Error("no done line with the report")
	}
	if len(e.gh.Reopened) != 0 || countMarker(e.comments(), "huddle:auto-handoff") != 0 {
		t.Errorf("reopened %v, comments %q; want neither", e.gh.Reopened, e.comments())
	}
	if p := e.phases(); p[len(p)-1] != PhaseDone {
		t.Errorf("last phase = %v, want PhaseDone", p[len(p)-1])
	}
}

func TestClosedDuringARunWithWorkOpenIsReopened(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.gh.Milestone = []gh.IssueState{
		{Number: 1, Closed: true},
		{Number: 4},
		{Number: 5, Body: "Spec in docs/x.md"},
		{Number: controlNo, Body: gh.ControlMarker}, // found by marker, never counts
	}
	e.agents.steps = []func(context.Context, call) Result{
		func(context.Context, call) Result {
			// The Dev hands over and closes the control issue by mistake.
			e.agentSets(gh.StatePMReady)
			e.gh.Issues[0].Closed = true
			return Result{SessionID: "dev-1"}
		},
		func(context.Context, call) Result {
			if e.gh.Issues[0].Closed {
				t.Error("the PM runs on a closed control issue")
			}
			e.agentSets(gh.StateDone)
			return Result{SessionID: "pm-1"}
		},
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !slices.Equal(e.gh.Reopened, []int{controlNo}) {
		t.Errorf("reopened %v, want [3]", e.gh.Reopened)
	}
	c := e.comments()
	if countMarker(c, reopenedMarker) != 1 || !strings.Contains(strings.Join(c, "\n"), "because issues #4, #5 are still open") {
		t.Errorf("comments = %q, want one reopen comment naming #4, #5", c)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[1].role != session.PM {
		t.Errorf("calls = %+v, want the Dev then the PM (the label it had)", e.agents.calls)
	}
}

// Closed at the first read (as after the startup choice "Reopen", or
// closed between startup and the first poll) with work open: reopened too.
func TestClosedAtFirstReadWithWorkOpen(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.gh.Issues[0].Closed = true
	e.gh.Milestone = []gh.IssueState{{Number: 4}}
	e.agents.steps = []func(context.Context, call) Result{e.sets(gh.StateDone, "d")}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.gh.Reopened) != 1 || len(e.agents.calls) != 1 || e.agents.calls[0].role != session.Dev {
		t.Errorf("reopened %v, calls %+v; want reopened and the Dev's turn", e.gh.Reopened, e.agents.calls)
	}
}

// When every other issue of the milestone is closed and only the control
// issue is open, the milestone is not done: only state:done or closing the
// control issue finishes it.
func TestOnlyTheControlIssueOpenIsNotDone(t *testing.T) {
	e := newEnv(t, gh.StateDevReady)
	e.gh.Milestone = []gh.IssueState{
		{Number: 1, Closed: true},
		{Number: 2, Closed: true},
		{Number: controlNo, Labels: []string{"control", gh.StateDevReady}},
	}
	e.agents.steps = []func(context.Context, call) Result{
		e.sets(gh.StatePMReady, "d"),
		e.sets(gh.StateDone, "p"),
	}
	if code := e.run(); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if len(e.agents.calls) != 2 || e.agents.calls[1].role != session.PM {
		t.Errorf("calls = %+v, want the Dev and then the PM, who sets state:done", e.agents.calls)
	}
}
