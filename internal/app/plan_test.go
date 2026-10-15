package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
)

// planReply is a companion reply with a plan of two milestones and three
// issues, and text around the block.
const planReply = "Here is the plan we agreed. Press Agree and Huddle creates it.\n" +
	"<plan>\n" +
	"milestone: Milestone 4 · Maps and exports\n" +
	"description: Maps on every page, and CSV exports.\n" +
	"issue: Add the /export endpoint\n" +
	"body: GET /export returns a CSV of all items.\n" +
	"Acceptance: the header row names the columns.\n" +
	"issue: Add a map to the home page\n" +
	"body: The home page shows a MapLibre map.\n" +
	"milestone: Milestone 5 · Admin dashboard\n" +
	"issue: Admin page\n" +
	"body: /admin lists the users.\n" +
	"</plan>"

// planFixture is a run screen in plan mode with fakes behind it.
type planFixture struct {
	*fixture
	queue start.Queue
	begun int       // calls of cfg.Begin
	sent  []tea.Msg // messages passed to cfg.Send
}

// newPlanFixture returns the run screen in plan mode on an empty repo.
func newPlanFixture(t *testing.T) *planFixture {
	t.Helper()
	pf := &planFixture{fixture: &fixture{t: t, now: at(9, 0), gh: &gh.Fake{}, comp: &fakeCompanion{}}}
	pf.queue = start.NewQueue(t.TempDir(), testRepo)
	cfg := pf.config(t.TempDir())
	cfg.Milestone, cfg.Control.Number = "", 0
	cfg.Planning = true
	cfg.Queue = pf.queue
	cfg.Begin = func() { pf.begun++ }
	cfg.Send = func(msg tea.Msg) { pf.sent = append(pf.sent, msg) }
	pf.m = New(cfg)
	pf.send(sizeMsg(testWidth, testHeight))
	return pf
}

// plan talks to the companion until its plan card waits.
func (pf *planFixture) plan() {
	pf.t.Helper()
	pf.comp.replies = []string{"What should the next milestones do?", planReply}
	pf.keys("What should we do next?", "enter")
	pf.keys("Maps and exports first, then an admin dashboard. Create them.", "enter")
	if pf.m.card.kind != cardPlan {
		pf.t.Fatalf("no plan card; chat: %+v", pf.m.chat)
	}
}

func TestPlanModeScreens(t *testing.T) {
	pf := newPlanFixture(t)
	if !pf.m.companionOpen || pf.m.fetching {
		t.Errorf("plan mode: companion open %v, fetching %v", pf.m.companionOpen, pf.m.fetching)
	}
	checkGolden(t, "16-plan-mode", pf.m.View())
	pf.plan()
	checkGolden(t, "16b-plan-card", pf.m.View())
	if got := pf.comp.lastAsked().context; !strings.Contains(got, "Milestone: none yet (plan mode)") ||
		!strings.Contains(got, "State: plan mode: no milestone runs yet") {
		t.Errorf("context block in plan mode:\n%s", got)
	}

	// Show all: the full plan in the Activity slot; the card keeps waiting.
	pf.keys("tab", "right", "enter")
	if pf.m.view != viewPlan || pf.m.card.kind != cardPlan || pf.m.focus != focusActivity {
		t.Fatalf("after Show all: view %d, card %d, focus %d", pf.m.view, pf.m.card.kind, pf.m.focus)
	}
	checkGolden(t, "16c-plan-show-all", pf.m.View())
	pf.keys("esc")
	if pf.m.view != viewList {
		t.Errorf("esc did not go back to the list")
	}
}

func TestAgreeingAPlanCreatesAndQueuesIt(t *testing.T) {
	pf := newPlanFixture(t)
	pf.plan()
	pf.keys("tab", "enter") // Agree

	wantLog := []string{
		"milestone Milestone 4 · Maps and exports",
		"issue #1 Milestone 4 · Maps and exports: Add the /export endpoint",
		"issue #2 Milestone 4 · Maps and exports: Add a map to the home page",
		"issue #3 Milestone 4 · Maps and exports: Milestone 4 · Maps and exports Control",
		"milestone Milestone 5 · Admin dashboard",
		"issue #4 Milestone 5 · Admin dashboard: Admin page",
		"issue #5 Milestone 5 · Admin dashboard: Milestone 5 · Admin dashboard Control",
	}
	if !slices.Equal(pf.gh.Log, wantLog) {
		t.Errorf("GitHub calls:\n%q\nwant\n%q", pf.gh.Log, wantLog)
	}
	if body := pf.gh.Issues[0].Body; body != "GET /export returns a CSV of all items.\nAcceptance: the header row names the columns." {
		t.Errorf("issue body = %q", body)
	}
	queued, err := pf.queue.Load()
	if err != nil || !slices.Equal(queued, []int{3, 5}) {
		t.Errorf("queue = %v, %v; want [3 5]", queued, err)
	}
	if pf.begun != 1 {
		t.Errorf("Begin called %d times, want once", pf.begun)
	}
	// Every step went to the screen while the next one ran.
	if len(pf.sent) != len(wantLog) {
		t.Errorf("sent %d step events, want %d", len(pf.sent), len(wantLog))
	}
	if !strings.Contains(pf.lastEvent(), "Queued control issues #3, #5") {
		t.Errorf("last event = %q", pf.lastEvent())
	}
	if pf.m.card.kind != cardNone {
		t.Error("the card is still there")
	}
}

func TestABrokenPlanCreatesNothing(t *testing.T) {
	pf := newPlanFixture(t)
	pf.comp.replies = []string{"<plan>\nmilestone: M4\nissue: Export\n</plan>"}
	pf.keys("Plan it", "enter")
	if pf.m.card.kind != cardNone {
		t.Fatal("a broken plan became a card")
	}
	last := pf.m.chat[len(pf.m.chat)-1]
	if last.Who != companion.WhoHuddle || !strings.Contains(last.Text, `issue "Export" has no body`) ||
		!strings.Contains(last.Text, "Nothing was created") {
		t.Errorf("last chat entry = %+v", last)
	}
	if len(pf.gh.Log) != 0 {
		t.Errorf("GitHub calls: %q", pf.gh.Log)
	}
}

// A plan agreed while a milestone runs waits in the queue.
func TestPlanWhileAMilestoneRuns(t *testing.T) {
	f := newFixture(t)
	q := start.NewQueue(t.TempDir(), testRepo)
	f.m.cfg.Queue = q
	begun := false
	f.m.cfg.Begin = func() { begun = true }
	f.comp.replies = []string{planReply}
	f.keys("Plan the next two milestones", "enter", "tab", "enter")
	queued, _ := q.Load()
	if len(queued) != 2 || begun {
		t.Errorf("queue %v, begun %v; want two queued and no Begin", queued, begun)
	}
	f.keys("/status", "enter")
	if !strings.Contains(f.m.statusContent(), "#58 Milestone 3 now · next #61, #63") {
		t.Errorf("/status:\n%s", f.m.statusContent())
	}
}

func TestNextMilestoneAndBackToTheStartMenu(t *testing.T) {
	f := newFixture(t)
	f.send(MilestoneMsg{Milestone: "Milestone 4", Control: gh.Control{Client: f.gh, Repo: testRepo, Number: 61}})
	if f.m.cfg.Milestone != "Milestone 4" || f.m.cfg.Control.Number != 61 || !strings.Contains(f.m.projectPane(), "Milestone 4 · control #61") {
		t.Errorf("project pane = %q", f.m.projectPane())
	}
	if !strings.Contains(f.lastEvent(), "Started Milestone 4 · control #61 · autonomy 8") {
		t.Errorf("last event = %q", f.lastEvent())
	}

	f.send(StatusMsg{Phase: loop.PhaseDone, Turn: 9, Since: f.now})
	f.send(LoopDoneMsg{Code: 0, Finished: true})
	if badge, _ := f.m.badge(); badge != "Done" {
		t.Errorf("badge = %q", badge)
	}
	if !strings.Contains(f.helpText(), "any key start menu") {
		t.Errorf("help = %q", f.helpText())
	}
	cmd := f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !f.m.toStart || cmd == nil {
		t.Fatal("a key after the last milestone did not go back to the start menu")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("the screen did not quit")
	}
}

// doneAgent finishes every turn by setting state:done on the control
// issue that is being worked on. cancelAt cancels the run's context at
// that turn instead (as /quit would), to stop Huddle mid-queue.
type doneAgent struct {
	gh       *gh.Fake
	turns    []int // the control issue of each turn
	cancelAt int
	cancel   context.CancelFunc
}

func (a *doneAgent) Run(context.Context, session.Role, string, string, int, func(activity.Event)) (loop.Result, error) {
	for i := range a.gh.Issues {
		is := &a.gh.Issues[i]
		if !slices.Contains(is.Labels, gh.StateDevWorking) {
			continue
		}
		a.turns = append(a.turns, is.Number)
		if len(a.turns) == a.cancelAt {
			a.cancel()
			return loop.Result{Canceled: true}, nil
		}
		is.Labels = append(is.Labels, gh.StateDone)
	}
	return loop.Result{}, nil
}

// noSleep is a clock whose waits end at once with the context's error.
type noSleep struct{}

func (noSleep) Now() time.Time                                   { return at(9, 0) }
func (noSleep) Sleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

// recorder is a loop.Reporter that keeps the events.
type recorder struct{ events []string }

func (r *recorder) Event(e activity.Event) { r.events = append(r.events, e.Text) }
func (r *recorder) Status(loop.Status)     {}

// driveConfig returns a Config whose loops run agent against fake.
func driveConfig(fake *gh.Fake, q start.Queue, agent loop.Agents, sent *[]tea.Msg) Config {
	return Config{
		Repo:    testRepo,
		Control: gh.Control{Client: fake, Repo: testRepo},
		Queue:   q,
		Send:    func(msg tea.Msg) { *sent = append(*sent, msg) },
		NewLoop: func(c gh.Issue) *loop.Loop {
			return &loop.Loop{
				Control:   gh.Control{Client: fake, Repo: testRepo, Number: c.Number},
				Milestone: c.Milestone,
				Agents:    agent,
				Team:      agents.Team{Dev: agents.Choice{CLI: agents.Codex()}, PM: agents.Choice{CLI: agents.Claude()}},
				Sessions:  session.New(filepath.Dir(q.Path), testRepo, c.Number),
				Clock:     noSleep{},
			}
		},
		Controls: &loop.Controls{},
	}
}

// TestQueueRunsTheNextMilestone: after state:done the next queued
// milestone starts by itself, and the queue survives a restart.
func TestQueueRunsTheNextMilestone(t *testing.T) {
	fake := &gh.Fake{Issues: []gh.Issue{
		{Number: 10, Milestone: "M4", Labels: []string{"control", gh.StateDevReady}},
		{Number: 20, Milestone: "M5", Labels: []string{"control"}},
		{Number: 30, Milestone: "M6", Labels: []string{"control"}},
	}}
	home := t.TempDir()
	q := start.NewQueue(home, testRepo)
	if err := q.Add(10, 20, 30); err != nil {
		t.Fatal(err)
	}

	// First Huddle: M4 finishes, M5 starts, and Huddle is stopped during
	// M5's first turn.
	ctx, cancel := context.WithCancel(context.Background())
	agent := &doneAgent{gh: fake, cancelAt: 2, cancel: cancel}
	var sent []tea.Msg
	cfg := driveConfig(fake, q, agent, &sent)
	cfg.First = fake.Issues[0]
	code, finished := drive(ctx, cfg, &recorder{}, nil)
	if code != loop.ExitInterrupted || finished {
		t.Fatalf("first run: code %d, finished %v", code, finished)
	}
	if !slices.Equal(agent.turns, []int{10, 20}) {
		t.Errorf("turns = %v, want M4 then M5", agent.turns)
	}
	if queued, _ := q.Load(); !slices.Equal(queued, []int{20, 30}) {
		t.Errorf("queue after M4 = %v, want [20 30]", queued)
	}

	// Restart: the start menu offers M5 to resume (the queue says it is
	// first), and the rest of the queue runs without anyone.
	facts, err := start.Load(context.Background(), fake, testRepo, "~/sandbox", []int{20, 30})
	if err != nil || facts.Resume == nil || facts.Resume.Number != 20 {
		t.Fatalf("resume after restart = %+v, %v", facts.Resume, err)
	}
	agent = &doneAgent{gh: fake}
	sent = nil
	cfg = driveConfig(fake, start.NewQueue(home, testRepo), agent, &sent)
	cfg.First = *facts.Resume
	rec := &recorder{}
	code, finished = drive(context.Background(), cfg, rec, nil)
	if code != loop.ExitOK || !finished {
		t.Fatalf("second run: code %d, finished %v; events %q", code, finished, rec.events)
	}
	if !slices.Equal(agent.turns, []int{20, 30}) {
		t.Errorf("turns = %v, want M5 then M6", agent.turns)
	}
	if queued, _ := q.Load(); len(queued) != 0 {
		t.Errorf("queue at the end = %v, want empty", queued)
	}
	var started []int
	for _, msg := range sent {
		if m, ok := msg.(MilestoneMsg); ok {
			started = append(started, m.Control.Number)
		}
	}
	if !slices.Equal(started, []int{20, 30}) {
		t.Errorf("milestones started on the screen = %v", started)
	}
}

// TestDriveInPlanModeWaitsForBegin: in plan mode nothing runs until a plan
// is agreed; then the first queued milestone gets state:dev-ready and runs.
func TestDriveInPlanModeWaitsForBegin(t *testing.T) {
	fake := &gh.Fake{Issues: []gh.Issue{{Number: 3, Milestone: "M4", Labels: []string{"control"}}}}
	q := start.NewQueue(t.TempDir(), testRepo)
	if err := q.Add(3); err != nil {
		t.Fatal(err)
	}
	agent := &doneAgent{gh: fake}
	var sent []tea.Msg
	cfg := driveConfig(fake, q, agent, &sent)
	cfg.Planning = true
	begin := make(chan struct{}, 1)
	begin <- struct{}{}
	code, finished := drive(context.Background(), cfg, &recorder{}, begin)
	if code != loop.ExitOK || !finished || !slices.Equal(agent.turns, []int{3}) {
		t.Errorf("code %d, finished %v, turns %v", code, finished, agent.turns)
	}

	// Without a plan, /quit ends the wait.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code, _ := drive(ctx, cfg, &recorder{}, make(chan struct{})); code != loop.ExitInterrupted {
		t.Errorf("code %d, want interrupted", code)
	}
}
