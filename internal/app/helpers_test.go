package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

func TestMain(m *testing.M) {
	// Without colours, a rendered screen is plain text that a golden file
	// can hold and a person can read.
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

const (
	testRepo    = "sohaibseniorswe-sys/palantir"
	testRepoDir = "/home/u/projects/palantir"
	testWidth   = 100
	testHeight  = 34
)

// fixture is a run screen in a known state, with fakes behind it.
type fixture struct {
	t       *testing.T
	m       Model
	now     time.Time // what the model's clock returns
	gh      *gh.Fake
	stopped bool   // cfg.Stop was called
	logPath string // the fake raw log of turn 5
	log     string // its content
	comp    *fakeCompanion
	history companion.History
}

// fakeCompanion answers with the replies it is given, in order, and
// records what it was asked.
type fakeCompanion struct {
	replies []string
	err     error
	asked   []asked
}

// asked is one message the fake companion received.
type asked struct {
	context string // the HUDDLE CONTEXT block
	message string
}

func (c *fakeCompanion) Ask(_ context.Context, contextBlock, message string) (string, error) {
	c.asked = append(c.asked, asked{context: contextBlock, message: message})
	if c.err != nil {
		return "", c.err
	}
	if len(c.replies) == 0 {
		return "OK.", nil
	}
	reply := c.replies[0]
	c.replies = c.replies[1:]
	return reply, nil
}

// lastAsked returns the newest message the fake companion received.
func (c *fakeCompanion) lastAsked() asked {
	if len(c.asked) == 0 {
		return asked{}
	}
	return c.asked[len(c.asked)-1]
}

// The conversation of screens 02 to 08 in docs/v3/screens.
var talk = []struct{ you, companion string }{
	{"Why JSON and not CSV for the export?", "The PM decided that at 23:03, after Codex asked in its handover on #58. The dashboard in #64 reads JSON, so CSV would need a second parser."},
	{"Could we add CSV later?", "Yes, as a small follow-up issue. Codex is on #62 now and CI for PR #73 is running. Want me to draft a decision?"},
}

// talk has the conversation of screens 02 to 08 with the companion, which
// opens the Companion pane.
func (f *fixture) talk() {
	f.t.Helper()
	for _, t := range talk {
		f.comp.replies = append(f.comp.replies, t.companion)
		f.keys(t.you, "enter")
	}
}

// at returns 2026-10-13 at hh:mm UTC.
func at(hh, mm int) time.Time { return time.Date(2026, 10, 13, hh, mm, 0, 0, time.UTC) }

// rawLog is a Dev run's log as runner.Run writes it: every line stamped.
const rawLog = `20:40:00  # huddle: codex exec --strict-config - < (prompt, 1926 bytes) in /home/u/projects/palantir at 2026-10-13T20:40:00Z
22:51:58  exec
22:51:58  /bin/bash -lc "go vet ./... && go test ./..." in /home/u/projects/palantir
22:52:01   succeeded in 2804ms:
22:52:01  ok      github.com/sohaibseniorswe-sys/palantir/internal/export  0.011s
22:52:07  exec
22:52:07  /bin/bash -lc "git push -u origin agent/issue-62 && gh pr create --title 'Add /export'
22:52:07    --body 'Closes #62'" in /home/u/projects/palantir
22:52:11   succeeded in 4231ms:
22:52:11   * [new branch]      agent/issue-62 -> agent/issue-62
22:52:11  https://github.com/sohaibseniorswe-sys/palantir/pull/73
22:52:12
22:52:12  codex
22:52:12  Waiting for CI on PR #73
22:52:13
22:52:13  exec
22:52:13  /bin/bash -lc 'gh pr checks 73 --watch --interval 10' in /home/u/projects/palantir
22:52:23  test    pending 0   https://github.com/sohaibseniorswe-sys/palantir/actions/runs/38461
22:52:43  test    pass    31s https://github.com/sohaibseniorswe-sys/palantir/actions/runs/38461
`

// newFixture returns the run screen of screen 01 in docs/v3/screens: the
// Dev is working on Milestone 3 and twelve events have arrived.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, now: at(20, 39)}
	f.gh = &gh.Fake{
		Milestone: []gh.IssueState{
			{Number: 59, Closed: true}, {Number: 60, Closed: true}, {Number: 61, Closed: true},
			{Number: 62}, {Number: 63, Closed: true}, {Number: 64}, {Number: 65},
		},
		PullRequests: []gh.PullRequest{{Number: 73, Title: "export endpoint", Checks: gh.Checks{Total: 2, Done: 1, Pending: 1}}},
		Merged:       &gh.PullRequest{Number: 71, MergedAt: at(22, 41)},
		MainRun:      &gh.Run{Status: "completed", Conclusion: "success"},
		Issues:       []gh.Issue{{Number: 58, Labels: []string{"control", gh.StateDevWorking}}},
	}

	logDir := t.TempDir()
	f.log = rawLog
	f.logPath = filepath.Join(logDir, "20261013-204000-turn5-dev.log")
	if err := os.WriteFile(f.logPath, []byte(f.log), 0o644); err != nil {
		t.Fatal(err)
	}
	// A log of another repo, which /log must leave out.
	other := "20:00:00  # huddle: codex exec - in /home/u/projects/other at 2026-10-13T20:00:00Z\n"
	if err := os.WriteFile(filepath.Join(logDir, "20261013-200000-turn1-dev.log"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}

	f.comp = &fakeCompanion{}
	f.history = companion.History{Path: filepath.Join(t.TempDir(), "companion.jsonl")}
	f.m = New(f.config(logDir))
	f.send(tea.WindowSizeMsg{Width: testWidth, Height: testHeight})
	f.send(StatusMsg{Phase: loop.PhaseWorking, Role: session.Dev, Turn: 5, Since: at(22, 41)})
	f.run(f.m.fetchFacts()) // what Init starts

	ref := func(line string) activity.LogRef {
		i := strings.Index(f.log, line)
		if i < 0 {
			t.Fatalf("line %q not in the log", line)
		}
		return activity.LogRef{Path: f.logPath, Offset: int64(i)}
	}
	for _, e := range []struct {
		hh, mm int
		actor  string
		text   string
		ref    activity.LogRef
	}{
		{20, 39, "huddle", "Started Milestone 3 · control #58", activity.LogRef{}},
		{20, 40, "codex", "Read #58, AGENTS.md and the 7 milestone issues", activity.LogRef{}},
		{20, 58, "codex", "Merged PR #68, closed #59", activity.LogRef{}},
		{21, 31, "codex", "Merged PR #69, closed #60", activity.LogRef{}},
		{22, 6, "codex", "Merged PR #70, closed #63", activity.LogRef{}},
		{22, 41, "codex", "Merged PR #71, closed #61", activity.LogRef{}},
		{22, 41, "codex", "Starting #62: add the /export endpoint", activity.LogRef{}},
		{22, 43, "codex", "$ go vet ./... && go test ./...   ok", ref("22:51:58  /bin/bash")},
		{22, 47, "codex", "Added export handler and table-driven tests", activity.LogRef{}},
		{22, 52, "codex", "$ git push -u origin agent/issue-62 && gh pr create --title 'Add /export'   ok", ref("22:52:07  exec")},
		{22, 52, "codex", "Waiting for CI on PR #73", ref("22:52:12  Waiting")},
		{22, 53, "huddle", "Usage check: Codex ok, Claude Code ok", activity.LogRef{}},
	} {
		f.send(EventMsg{Time: at(e.hh, e.mm), Actor: e.actor, Text: e.text, LogRef: e.ref})
	}
	f.now = at(22, 53)
	f.send(tickMsg(f.now)) // its command (the next tick and a fetch) is not run
	return f
}

// config returns the run screen's Config, with the fixture's fakes.
func (f *fixture) config(logDir string) Config {
	return Config{
		Repo:      testRepo,
		Milestone: "Milestone 3",
		Control:   gh.Control{Client: f.gh, Repo: testRepo, Number: 58},
		Controls:  &loop.Controls{},
		Stop:      func() { f.stopped = true },
		Team: agents.Team{
			Dev:       agents.Choice{CLI: agents.Codex()},
			PM:        agents.Choice{CLI: agents.Claude()},
			Companion: agents.Choice{CLI: agents.Claude()},
		},
		Settings:  Settings{Autonomy: 8},
		LogDir:    logDir,
		RepoDir:   testRepoDir,
		Now:       func() time.Time { return f.now },
		Companion: f.comp,
		History:   f.history,
	}
}

// send passes msg to Update and keeps the new model. It returns the
// command Update returned.
func (f *fixture) send(msg tea.Msg) tea.Cmd {
	f.t.Helper()
	next, cmd := f.m.Update(msg)
	f.m = next.(Model)
	return cmd
}

// run runs cmd, as Bubble Tea would in the background, and sends its
// message. Commands made of several (tea.Batch) are run one by one. It
// must not be given a tick's command: that one waits for its timer.
func (f *fixture) run(cmd tea.Cmd) {
	f.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			f.run(c)
		}
	case tickMsg, spinner.TickMsg:
		// A timer: not run in tests.
	default:
		f.run(f.send(msg)) // and whatever that message starts in turn
	}
}

// keys types each key: special keys by name ("up", "enter", "tab", "esc",
// "ctrl+c", "pgup", "left", "backspace"), anything else as text.
func (f *fixture) keys(keys ...string) {
	f.t.Helper()
	special := map[string]tea.KeyType{
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
		"enter": tea.KeyEnter, "tab": tea.KeyTab, "esc": tea.KeyEsc, "ctrl+c": tea.KeyCtrlC,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "backspace": tea.KeyBackspace,
	}
	for _, k := range keys {
		if t, ok := special[k]; ok {
			f.run(f.send(tea.KeyMsg{Type: t}))
			continue
		}
		for _, r := range k {
			f.run(f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
		}
	}
}

// eventTexts returns the texts of the events, newest last.
func (f *fixture) eventTexts() []string {
	var out []string
	for _, e := range f.m.events {
		out = append(out, e.Text)
	}
	return out
}

// lastEvent returns the newest event's text.
func (f *fixture) lastEvent() string {
	if len(f.m.events) == 0 {
		return ""
	}
	return f.m.events[len(f.m.events)-1].Text
}

// sizeMsg is a terminal resize to width × height.
func sizeMsg(width, height int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// newDecisionFixture returns the run screen of screen 10: the PM needs a
// Mapbox token, the companion explained it, the human chose, a decision
// card waits, and the human is typing another question.
func newDecisionFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.m.events, f.m.rendered = nil, nil
	for _, e := range []struct {
		hh, mm int
		actor  string
		text   string
	}{
		{23, 41, "codex", "Merged PR #74, closed #64"},
		{23, 58, "codex", "Closed #66; #65 blocked: no MAPBOX_TOKEN"},
		{23, 59, "codex", "Handed over to PM (Decision needed)"},
		{0, 2, "pm", "Needs you: #65 needs a Mapbox token"},
	} {
		f.send(EventMsg{Time: at(e.hh, e.mm), Actor: e.actor, Text: e.text})
	}
	f.gh.Issues[0].Labels = []string{"control", gh.StateHumanRequired}
	f.gh.IssueComments = map[int][]gh.Comment{58: {{Author: "sohaibseniorswe-sys", Body: "Status: Decision needed\n#65 needs a Mapbox token only the human can create."}}}
	for i := range f.gh.Milestone[:6] {
		f.gh.Milestone[i].Closed = true
	}
	f.run(f.m.fetchFacts())
	f.now = at(0, 2)
	f.comp.replies = []string{
		"The PM needs a token only you can create. Options: Mapbox (token), MapLibre with free OpenStreetMap tiles (no key), or skip the map for now.",
		"MapLibre with OSM tiles needs no key and keeps #65's acceptance criteria. Here's the decision:\n" +
			"<decision>Use MapLibre with OpenStreetMap tiles for #65 now (no API key). Add a follow-up issue in Milestone 4 to switch to Mapbox once the token exists.</decision>\n" +
			"<reason>no Mapbox token yet; MapLibre keeps #65's acceptance criteria.</reason>",
	}
	f.run(f.send(StatusMsg{Phase: loop.PhaseHuman, Turn: 6, Since: f.now}))
	f.keys("Use OpenStreetMap for now. I'll get a Mapbox token next week.", "enter")
	f.keys("Is MapLibre as fast as Mapbox on mobile?")
	return f
}

// helpText is the help line as plain text.
func (f *fixture) helpText() string {
	return ansi.Strip(f.m.helpLine())
}

// newSettingsFixture returns the run screen of newFixture with a home
// folder holding the project's settings, logs and sessions, as /settings
// reads them: Codex as the Dev with the CLI's default model, Claude Code as
// the PM with opus at high effort, the companion following the PM.
func newSettingsFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	home := t.TempDir()
	logDir := filepath.Join(home, ".huddle", "logs")
	logs := map[string]string{
		"20261013-204000-turn5-dev.log": "20:40:00  # huddle: codex exec --strict-config - in " + testRepoDir + " at 2026-10-13T20:40:00Z\n" +
			"20:40:01  OpenAI Codex v0.160.0\n20:40:01  model: gpt-6.1-sol\n20:40:01  session id: 01a13bd6-ec1a-7580-8fef-eaf9c06ef981\n",
		"20261013-223000-turn4-pm.log": "22:30:00  # huddle: claude -p --output-format json --model opus --effort high in " + testRepoDir + " at 2026-10-13T22:30:00Z\n" +
			`22:39:00  {"type":"result","is_error":false,"session_id":"717c4e2a-0000-4000-8000-0000000002f5","result":"Over to Codex.","modelUsage":{"claude-opus-5-5":{"outputTokens":800}}}` + "\n",
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range logs {
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sessions := session.New(home, testRepo, 58)
	if err := sessions.Save(session.Dev, "01a13bd6-ec1a-7580-8fef-eaf9c06ef981"); err != nil {
		t.Fatal(err)
	}
	if err := sessions.Save(session.PM, "717c4e2a-0000-4000-8000-0000000002f5"); err != nil {
		t.Fatal(err)
	}

	f.m.cfg.LogDir = logDir
	f.m.cfg.Team.PM = agents.Choice{CLI: agents.Claude(), Model: "opus", Effort: "high"}
	f.m.cfg.Team.Companion = f.m.cfg.Team.PM
	f.m.cfg.Settings = Settings{
		Home:               home,
		Path:               filepath.Join(home, ".huddle", "projects", "sohaibseniorswe-sys_palantir.yaml"),
		Exists:             true,
		Autonomy:           8,
		CompanionFollowsPM: true,
		Versions:           map[string]string{agents.CodexID: "0.160.0", agents.ClaudeID: "2.1.299"},
		Sessions:           sessions,
		CompanionSessions:  session.NewCompanion(home, testRepo, 58),
	}
	return f
}
