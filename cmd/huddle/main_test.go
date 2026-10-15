package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/app"
	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
	"github.com/sohaibseniorswe-sys/huddle/internal/wizard"
)

const sandbox = "sohaibseniorswe-sys/huddle-sandbox"

// stopClock ends the watch loop at its first poll, as Ctrl+C would.
type stopClock struct{}

func (stopClock) Now() time.Time { return time.Now() }

func (stopClock) Sleep(context.Context, time.Duration) error { return context.Canceled }

// noAgents fails the test if an agent runs.
type noAgents struct{ t *testing.T }

func (a noAgents) Run(context.Context, session.Role, string, string, int, func(activity.Event)) (loop.Result, error) {
	a.t.Error("an agent ran")
	return loop.Result{}, nil
}

// fakeAgent records runs and ends each turn by setting label.
type fakeAgent struct {
	gh    *gh.Fake
	label string
	runs  []session.Role
}

func (a *fakeAgent) Run(_ context.Context, role session.Role, _, _ string, _ int, _ func(activity.Event)) (loop.Result, error) {
	a.runs = append(a.runs, role)
	a.gh.Issues[0].Labels = append(a.gh.Issues[0].Labels, a.label)
	return loop.Result{SessionID: "sess-" + string(role)}, nil
}

// testDeps returns deps whose external calls all succeed, rooted at dir.
// HOME is a temp dir unless env sets it.
func testDeps(t *testing.T, dir string, env map[string]string, stdin string) (deps, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	home := t.TempDir()
	return deps{
		stdin:  strings.NewReader(stdin),
		stdout: &stdout,
		stderr: &stderr,
		getwd:  func() (string, error) { return dir, nil },
		lookupEnv: func(k string) (string, bool) {
			if v, ok := env[k]; ok {
				return v, ok
			}
			if k == "HOME" {
				return home, true
			}
			return "", false
		},
		gh: &gh.Fake{
			Issues: []gh.Issue{{
				Number: 3, Title: "Sandbox Control", Milestone: "Sandbox M1",
				Labels: []string{"control"},
			}},
			Milestones: []gh.Milestone{{Number: 1, Title: "Sandbox M1", OpenIssues: 2}},
		},
		agents: checks.AgentCLIs{
			LookPath: func(string) (string, error) { return "/bin/true", nil },
			Run: func(_ context.Context, name string, _ ...string) (string, string, error) {
				if name == "codex" {
					return "", "Logged in using ChatGPT\n", nil
				}
				return `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}`, "", nil
			},
		},
		runAgents: func(string, string, agents.Team, io.Writer) loop.Agents { return noAgents{t} },
		clock:     stopClock{},
		// The tests never open the full-screen UI: this stand-in runs the
		// first milestone's loop with plain output instead (plan mode has
		// none, and returns 0).
		runUI: func(ctx context.Context, cfg app.Config, _ io.Writer) (int, bool) {
			if cfg.Planning {
				return 0, false
			}
			l := cfg.NewLoop(cfg.First)
			l.Report = loop.PlainReporter{Out: &stdout}
			l.Controls = &loop.Controls{}
			return l.Run(ctx), false
		},
		// Nor the start menu: this stand-in quits, as Esc would.
		runStart: func(context.Context, start.Facts, string) (start.Choice, error) {
			return start.Choice{Kind: start.KindQuit}, nil
		},
		// Nor the wizard: this stand-in saves the settings it is given,
		// as a human pressing Enter at every step would.
		runWizard: func(_ context.Context, wd wizard.Deps, c config.Config) (config.Config, error) {
			return c, config.Save(wd.Home, c)
		},
		validate: func(context.Context, agents.CLI, string, string) error { return nil },
	}, &stdout, &stderr
}

// gitRepo creates a git repo in a temp dir with the given origin and agent files.
func gitRepo(t *testing.T, origin string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", origin},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	for name, content := range map[string]string{
		"AGENTS.md":  "rules\n",
		"CLAUDE.md":  "Read and follow AGENTS.md and Session.md.\n",
		".gitignore": "Session.md\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestVersion(t *testing.T) {
	d, stdout, _ := testDeps(t, t.TempDir(), nil, "")
	if code := run(context.Background(), []string{"--version"}, d); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(stdout.String(), "huddle ") {
		t.Errorf("--version printed %q", stdout.String())
	}
}

func TestRepoFlagValidation(t *testing.T) {
	for _, bad := range []string{"palantir", "a/b/c", "https://github.com/a/b", "", "owner/"} {
		d, _, stderr := testDeps(t, t.TempDir(), nil, "")
		if code := run(context.Background(), []string{"--repo", bad}, d); code != 2 {
			t.Errorf("--repo %q: exit %d, want 2", bad, code)
		}
		if !strings.Contains(stderr.String(), "--repo") {
			t.Errorf("--repo %q: stderr %q does not mention --repo", bad, stderr.String())
		}
	}
}

func TestUnexpectedArgs(t *testing.T) {
	d, _, _ := testDeps(t, t.TempDir(), nil, "")
	if code := run(context.Background(), []string{"extra"}, d); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

func TestRefusesWithAPIKeys(t *testing.T) {
	for _, key := range checks.APIKeyVars {
		t.Run(key, func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox+".git")
			d, stdout, stderr := testDeps(t, dir, map[string]string{key: "sk-test"}, "y\n")
			if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
				t.Fatalf("exit %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), key) {
				t.Errorf("stderr %q does not name %s", stderr.String(), key)
			}
			if strings.Contains(stdout.String(), "✔") {
				t.Errorf("checks ran despite API key: %q", stdout.String())
			}
		})
	}
}

func TestStartupSummary(t *testing.T) {
	dir := gitRepo(t, "git@github.com:"+sandbox+".git")
	// A dirty tree is fine: after an interrupted run it normally is.
	os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("x"), 0o644)
	d, stdout, stderr := testDeps(t, dir, map[string]string{}, "\n")
	saveSettings(t, d, sandbox)
	// No state label: the loop prints the hint and polls; stopClock ends it.
	if code := run(context.Background(), []string{"--repo", sandbox, "--plain"}, d); code != loop.ExitInterrupted {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"target: " + sandbox + " (override)",
		"created empty Session.md",
		"labels ready (created control, state:dev-ready",
		`control issue #3 "Sandbox Control", milestone "Sandbox M1", state: none`,
		"Continue? [Y/n]",
		"add state:dev-ready to start",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Session.md")); err != nil {
		t.Errorf("Session.md not created: %v", err)
	}
}

func TestStartupDeclined(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, _ := testDeps(t, dir, nil, "n\n")
	saveSettings(t, d, sandbox)
	if code := run(context.Background(), []string{"--repo", sandbox, "--plain"}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("output %q missing 'aborted'", stdout.String())
	}
}

func TestStartupResumeOnceRunsOneTurn(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, stderr := testDeps(t, dir, nil, "")
	fake := d.gh.(*gh.Fake)
	fake.Issues[0].Labels = append(fake.Issues[0].Labels, gh.StateDevReady)
	agent := &fakeAgent{gh: fake, label: gh.StatePMReady}
	var gotDir, gotHome string
	d.runAgents = func(dir, home string, _ agents.Team, _ io.Writer) loop.Agents {
		gotDir, gotHome = dir, home
		return agent
	}
	if code := run(context.Background(), []string{"--repo", sandbox, "--resume", "--once"}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Continue?") {
		t.Error("--resume prompted")
	}
	if len(agent.runs) != 1 || agent.runs[0] != session.Dev {
		t.Errorf("runs = %v, want one Dev turn", agent.runs)
	}
	if real, _ := filepath.EvalSymlinks(dir); gotDir != real && gotDir != dir {
		t.Errorf("agents run in %q, want the repo root %q", gotDir, dir)
	}
	home, _ := d.lookupEnv("HOME")
	if gotHome != home {
		t.Errorf("home = %q", gotHome)
	}
	store := session.New(home, sandbox, 3)
	if got := store.Load().Dev; got != "sess-dev" {
		t.Errorf("saved dev session = %q, want it under HOME/.huddle/sessions", got)
	}
	if got := checks.States(fake.Issues[0]); len(got) != 1 || got[0] != "pm-ready" {
		t.Errorf("states = %v", got)
	}
}

func TestStartupFailures(t *testing.T) {
	t.Run("default target, sandbox clone", func(t *testing.T) {
		dir := gitRepo(t, "https://github.com/"+sandbox)
		d, _, stderr := testDeps(t, dir, nil, "y\n")
		if code := run(context.Background(), nil, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), checks.DefaultRepo) {
			t.Errorf("stderr %q", stderr.String())
		}
	})
	t.Run("not a git repo", func(t *testing.T) {
		d, _, stderr := testDeps(t, t.TempDir(), nil, "y\n")
		if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "not inside a git repo") {
			t.Errorf("stderr %q", stderr.String())
		}
	})
	t.Run("gh not logged in", func(t *testing.T) {
		dir := gitRepo(t, "https://github.com/"+sandbox)
		d, _, stderr := testDeps(t, dir, nil, "y\n")
		d.gh = &gh.Fake{AuthErr: errors.New("not logged in")}
		if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "gh auth login") {
			t.Errorf("stderr %q", stderr.String())
		}
	})
}

func TestTildePath(t *testing.T) {
	env := func(k string) (string, bool) { return "/home/sweeng", k == "HOME" }
	if got := tildePath("/home/sweeng/projects/palantir", env); got != "~/projects/palantir" {
		t.Errorf("tildePath() = %q", got)
	}
	if got := tildePath("/srv/palantir", env); got != "/srv/palantir" {
		t.Errorf("tildePath() = %q", got)
	}
}

func TestRunScreenOrPlain(t *testing.T) {
	for _, tt := range []struct {
		args       []string
		wantScreen bool
	}{
		{[]string{"--repo", sandbox, "--resume"}, true},
		{[]string{"--repo", sandbox, "--yes", "--plain"}, false},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox)
			d, stdout, _ := testDeps(t, dir, nil, "")
			saveSettings(t, d, sandbox)
			if tt.wantScreen {
				fake := d.gh.(*gh.Fake)
				fake.Issues[0].Labels = append(fake.Issues[0].Labels, gh.StateDevReady)
			}
			var stream io.Writer = &bytes.Buffer{} // anything but nil
			d.runAgents = func(_, _ string, _ agents.Team, s io.Writer) loop.Agents { stream = s; return noAgents{t} }
			var cfg app.Config
			screen := false
			d.runUI = func(_ context.Context, c app.Config, _ io.Writer) (int, bool) {
				screen, cfg = true, c
				c.NewLoop(c.First) // the loop's agents are made as the screen would
				return 0, false
			}
			run(context.Background(), tt.args, d)
			if screen != tt.wantScreen {
				t.Fatalf("run screen shown = %t, want %t", screen, tt.wantScreen)
			}
			if !tt.wantScreen {
				if stream != d.stdout {
					t.Error("--plain: the agents' raw output does not go to stdout")
				}
				if !strings.Contains(stdout.String(), "huddle: control issue #3 has no state:* label") {
					t.Errorf("--plain output misses the loop's lines:\n%s", stdout)
				}
				return
			}
			if stream != nil {
				t.Error("run screen: the agents' raw output must not reach the terminal")
			}
			home, _ := d.lookupEnv("HOME")
			if cfg.Repo != sandbox || cfg.Milestone != "Sandbox M1" || cfg.Control.Number != 3 ||
				cfg.LogDir != filepath.Join(home, ".huddle", "logs") || cfg.Now == nil {
				t.Errorf("run screen config = %+v", cfg)
			}
		})
	}
}

// TestSettingsFileChoosesTheAgents runs one turn with a settings file that
// swaps the roles: the loop's agents get the team from the file, and only
// the CLIs in use are checked.
func TestSettingsFileChoosesTheAgents(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, stderr := testDeps(t, dir, nil, "")
	home, _ := d.lookupEnv("HOME")
	c := config.Default(sandbox)
	c.Agents.Dev = config.Agent{CLI: "claude", Model: "sonnet", Effort: "high"}
	c.Agents.PM = config.Agent{CLI: "claude", Model: "opus"}
	if err := config.Save(home, c); err != nil {
		t.Fatal(err)
	}
	var checked []string
	d.agents.LookPath = func(bin string) (string, error) {
		checked = append(checked, bin)
		return "/bin/true", nil
	}
	fake := d.gh.(*gh.Fake)
	fake.Issues[0].Labels = append(fake.Issues[0].Labels, gh.StateDevReady)
	agent := &fakeAgent{gh: fake, label: gh.StatePMReady}
	var team agents.Team
	d.runAgents = func(_, _ string, t agents.Team, _ io.Writer) loop.Agents { team = t; return agent }

	if code := run(context.Background(), []string{"--repo", sandbox, "--yes", "--once", "--plain"}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if team.Dev.CLI.ID != "claude" || team.Dev.Model != "sonnet" || team.Dev.Effort != "high" || team.PM.Model != "opus" {
		t.Errorf("team = %+v", team)
	}
	if strings.Join(checked, ",") != "claude" {
		t.Errorf("checked CLIs %v, want only claude: codex plays no role", checked)
	}
	for _, want := range []string{"✔ settings ~/.huddle/projects/sohaibseniorswe-sys_huddle-sandbox.yaml",
		"Dev: Claude Code · sonnet · high; PM: Claude Code · opus · CLI default effort",
		"turn 1: Dev (Claude Code), fresh session"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, stdout.String())
		}
	}
}

func TestInvalidSettingsStopTheStart(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, _, stderr := testDeps(t, dir, nil, "")
	home, _ := d.lookupEnv("HOME")
	c := config.Default(sandbox)
	c.Agents.PM.Effort = "none" // a Codex effort, not Claude Code's
	if err := config.Save(home, c); err != nil {
		t.Fatal(err)
	}
	d.runAgents = func(string, string, agents.Team, io.Writer) loop.Agents { return noAgents{t} }
	if code := run(context.Background(), []string{"--repo", sandbox, "--yes"}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), `agents.pm.effort: "none" is not a Claude Code effort`) {
		t.Errorf("stderr = %q", stderr.String())
	}
}

// saveSettings writes the default settings for repo under d's HOME, so the
// wizard does not run.
func saveSettings(t *testing.T, d deps, repo string) {
	t.Helper()
	home, _ := d.lookupEnv("HOME")
	if err := config.Save(home, config.Default(repo)); err != nil {
		t.Fatal(err)
	}
}

func TestFirstRunOpensTheWizardThenSkipsIt(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, stderr := testDeps(t, dir, nil, "")
	calls := 0
	save := d.runWizard
	d.runWizard = func(ctx context.Context, wd wizard.Deps, c config.Config) (config.Config, error) {
		calls++
		if wd.Target != sandbox || wd.Home == "" || wd.GH == nil || wd.Validate == nil || wd.CheckCLI == nil {
			t.Errorf("wizard deps = %+v", wd)
		}
		return save(ctx, wd, c)
	}
	args := []string{"--repo", sandbox, "--yes", "--plain"}
	if code := run(context.Background(), []string{"--repo", sandbox, "--plan"}, d); code != 0 {
		t.Fatalf("first run: exit %d, stderr: %s", code, stderr.String())
	}
	if calls != 1 || !strings.Contains(stdout.String(), "✔ settings saved to ~/.huddle/projects/sohaibseniorswe-sys_huddle-sandbox.yaml") {
		t.Fatalf("wizard ran %d times; output:\n%s", calls, stdout)
	}
	// The next start finds the file: no wizard, even with --plain.
	if code := run(context.Background(), args, d); code != loop.ExitInterrupted {
		t.Fatalf("second run: exit %d, stderr: %s", code, stderr.String())
	}
	if calls != 1 {
		t.Errorf("wizard ran %d times, want once", calls)
	}
}

func TestSetupRunsTheWizardWithTheCurrentSettings(t *testing.T) {
	d, stdout, _ := testDeps(t, t.TempDir(), nil, "")
	home, _ := d.lookupEnv("HOME")
	c := config.Default(sandbox)
	c.Autonomy = 3
	c.Agents.Dev.Effort = "high"
	if err := config.Save(home, c); err != nil {
		t.Fatal(err)
	}
	var got config.Config
	d.runWizard = func(_ context.Context, _ wizard.Deps, c config.Config) (config.Config, error) {
		got = c
		return c, nil
	}
	// Flags after "setup" count too. setup exits once the settings are saved.
	if code := run(context.Background(), []string{"setup", "--repo", sandbox}, d); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got.Autonomy != 3 || got.Agents.Dev.Effort != "high" || got.Repo != sandbox {
		t.Errorf("wizard started from %+v, want the saved settings", got)
	}
	if !strings.Contains(stdout.String(), "settings saved") || strings.Contains(stdout.String(), "control issue") {
		t.Errorf("output = %q", stdout.String())
	}
}

func TestPlainWithoutSettings(t *testing.T) {
	for _, args := range [][]string{{"--repo", sandbox, "--plain"}, {"--plain", "setup"}} {
		d, _, stderr := testDeps(t, t.TempDir(), nil, "")
		d.runWizard = func(context.Context, wizard.Deps, config.Config) (config.Config, error) {
			t.Error("the wizard ran with --plain")
			return config.Config{}, nil
		}
		if code := run(context.Background(), args, d); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
		if stderr.String() != "Run huddle setup first\n" {
			t.Errorf("%v: stderr = %q", args, stderr.String())
		}
	}
}

func TestWizardCanceled(t *testing.T) {
	d, stdout, _ := testDeps(t, t.TempDir(), nil, "")
	d.runWizard = func(context.Context, wizard.Deps, config.Config) (config.Config, error) {
		return config.Config{}, wizard.ErrCanceled
	}
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "nothing was saved") {
		t.Errorf("output = %q", stdout.String())
	}
}

// A control issue closed while its milestone still has open issues: at
// startup Huddle asks whether to reopen it and continue, or leave it.
// After a reopen the PM (the label it had) runs and sets state:done.
func TestStartupClosedControlIssueWithWorkOpen(t *testing.T) {
	for _, tt := range []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantReopen bool
	}{
		{"reopen and continue", []string{"--repo", sandbox, "--resume"}, "1\n", 0, true},
		{"leave it and exit", []string{"--repo", sandbox, "--resume"}, "2\n", 0, false},
		{"--yes reopens", []string{"--repo", sandbox, "--yes", "--resume"}, "", 0, true},
		{"plain: reopen and confirm", []string{"--repo", sandbox, "--plain"}, "1\n\n", 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox)
			d, stdout, stderr := testDeps(t, dir, nil, tt.stdin)
			saveSettings(t, d, sandbox)
			fake := d.gh.(*gh.Fake)
			fake.Issues[0].Closed = true
			fake.Issues[0].Labels = append(fake.Issues[0].Labels, gh.StatePMReady)
			fake.Milestone = []gh.IssueState{{Number: 1, Closed: true}, {Number: 4}, {Number: 5}}
			d.clock = stopClock{}
			d.runAgents = func(string, string, agents.Team, io.Writer) loop.Agents {
				return &fakeAgent{gh: fake, label: gh.StateDone}
			}
			if code := run(context.Background(), tt.args, d); code != tt.wantCode {
				t.Fatalf("exit %d, want %d; stderr: %s", code, tt.wantCode, stderr.String())
			}
			out := stdout.String()
			if tt.stdin != "" && !strings.Contains(out, "Control issue #3 is closed but 2 issues are still open (#4, #5)") {
				t.Errorf("no question in the output:\n%s", out)
			}
			if reopened := len(fake.Reopened) == 1; reopened != tt.wantReopen {
				t.Errorf("reopened = %v, want %v; output:\n%s", reopened, tt.wantReopen, out)
			}
			if !tt.wantReopen && !strings.Contains(out, "left control issue #3 closed") {
				t.Errorf("output = %s", out)
			}
		})
	}
}

// With nothing left open, a closed control issue is a finished milestone,
// not a choice: Huddle reports that there is no open control issue.
func TestStartupClosedControlIssueNothingOpen(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, _, stderr := testDeps(t, dir, nil, "")
	saveSettings(t, d, sandbox)
	fake := d.gh.(*gh.Fake)
	fake.Issues[0].Closed = true
	fake.Milestone = []gh.IssueState{{Number: 1, Closed: true}}
	if code := run(context.Background(), []string{"--repo", sandbox, "--yes", "--plain"}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no open issue labelled") || len(fake.Reopened) != 0 {
		t.Errorf("stderr = %q, reopened %v", stderr.String(), fake.Reopened)
	}
}

// TestAutonomyFlag checks that --autonomy replaces the settings' level for
// one run (the loop and the run screen get it) without changing the file.
func TestAutonomyFlag(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want int
	}{
		{"settings level", nil, 6},
		{"flag overrides", []string{"--autonomy", "3"}, 3},
		{"zero is a level", []string{"--autonomy=0"}, 0},
		{"top of the dial", []string{"--autonomy", "10"}, 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox)
			d, stdout, stderr := testDeps(t, dir, nil, "")
			home, _ := d.lookupEnv("HOME")
			c := config.Default(sandbox)
			c.Autonomy = 6
			if err := config.Save(home, c); err != nil {
				t.Fatal(err)
			}
			d.runAgents = func(_, _ string, _ agents.Team, _ io.Writer) loop.Agents { return noAgents{t} }
			var loopLevel, screenLevel int
			d.runUI = func(_ context.Context, cfg app.Config, _ io.Writer) (int, bool) {
				loopLevel, screenLevel = cfg.NewLoop(cfg.First).Autonomy, cfg.Settings.Autonomy
				return 0, false
			}
			args := append([]string{"--repo", sandbox, "--plan"}, tt.args...)
			if code := run(context.Background(), args, d); code != 0 {
				t.Fatalf("exit %d, stderr: %s", code, stderr.String())
			}
			if loopLevel != tt.want || screenLevel != tt.want {
				t.Errorf("loop autonomy %d, run screen autonomy %d, want %d", loopLevel, screenLevel, tt.want)
			}
			if want := fmt.Sprintf("✔ autonomy %d · %s", tt.want, config.AutonomyName(tt.want)); !strings.Contains(stdout.String(), want) {
				t.Errorf("output lacks %q:\n%s", want, stdout.String())
			}
			if saved, _, _ := config.Load(home, sandbox); saved.Autonomy != 6 {
				t.Errorf("settings file autonomy = %d, want 6 (unchanged)", saved.Autonomy)
			}
		})
	}
}

func TestAutonomyFlagOutOfRange(t *testing.T) {
	for _, bad := range []string{"11", "-1", "100", "eight"} {
		d, stdout, stderr := testDeps(t, t.TempDir(), nil, "")
		if code := run(context.Background(), []string{"--repo", sandbox, "--autonomy", bad}, d); code != 2 {
			t.Errorf("--autonomy %s: exit %d, want 2", bad, code)
		}
		if !strings.Contains(stderr.String(), "autonomy") {
			t.Errorf("--autonomy %s: stderr %q does not mention the flag", bad, stderr.String())
		}
		if stdout.Len() > 0 {
			t.Errorf("--autonomy %s: startup ran: %q", bad, stdout.String())
		}
	}
	d, _, stderr := testDeps(t, t.TempDir(), nil, "")
	run(context.Background(), []string{"--autonomy", "11"}, d)
	if want := "huddle: --autonomy: 11 is outside 0–10"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
}

// TestStartMenuRunsAMilestone picks a milestone without a control issue in
// the start menu, with a first message: Huddle creates the control issue
// with that message as the First Dev task, labels it state:dev-ready and
// opens the run screen on it.
func TestStartMenuRunsAMilestone(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, _, stderr := testDeps(t, dir, nil, "")
	saveSettings(t, d, sandbox)
	fake := d.gh.(*gh.Fake)
	fake.Issues = []gh.Issue{{Number: 7, Title: "Export", Milestone: "M2"}}
	fake.Milestones = []gh.Milestone{{Number: 2, Title: "M2", OpenIssues: 1}}
	var facts start.Facts
	d.runStart = func(_ context.Context, f start.Facts, settings string) (start.Choice, error) {
		facts = f
		if !strings.Contains(settings, "none yet") {
			t.Errorf("settings page = %q", settings)
		}
		return start.Choice{Kind: start.KindRun, Milestone: f.Milestones[0], Message: "Start with #7."}, nil
	}
	var cfg app.Config
	d.runUI = func(_ context.Context, c app.Config, _ io.Writer) (int, bool) {
		cfg = c
		return 0, false
	}
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if len(facts.Milestones) != 1 || facts.Milestones[0].Control != 0 || facts.Resume != nil {
		t.Errorf("start menu facts = %+v", facts)
	}
	control := fake.Issues[len(fake.Issues)-1]
	if control.Number != 8 || control.Title != "M2 Control" || !strings.Contains(control.Body, "Prompt: Start with #7.") {
		t.Fatalf("control issue = %+v", control)
	}
	if got := checks.States(control); len(got) != 1 || got[0] != "dev-ready" {
		t.Errorf("states = %v", got)
	}
	if cfg.First.Number != 8 || cfg.Milestone != "M2" || cfg.Control.Number != 8 || cfg.Planning {
		t.Errorf("run screen config: first %+v, milestone %q, control #%d", cfg.First, cfg.Milestone, cfg.Control.Number)
	}
}

// TestStartMenuComesBack: when the run screen reports that every
// milestone is done (startMenu), the start menu opens again.
func TestStartMenuComesBack(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, _, _ := testDeps(t, dir, nil, "")
	saveSettings(t, d, sandbox)
	menus, screens := 0, 0
	d.runStart = func(context.Context, start.Facts, string) (start.Choice, error) {
		menus++
		if menus == 1 {
			return start.Choice{Kind: start.KindPlan}, nil
		}
		return start.Choice{Kind: start.KindQuit}, nil
	}
	d.runUI = func(_ context.Context, c app.Config, _ io.Writer) (int, bool) {
		screens++
		if !c.Planning {
			t.Error("not in plan mode")
		}
		return 0, true
	}
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if menus != 2 || screens != 1 {
		t.Errorf("start menu shown %d times, run screen %d; want 2 and 1", menus, screens)
	}
}

func TestStartFlags(t *testing.T) {
	for _, tt := range []struct {
		name     string
		args     []string
		wantCode int
		wantErr  string
	}{
		{"two modes", []string{"--resume", "--plan"}, 2, "only one of --resume, --run and --plan"},
		{"plan needs the screen", []string{"--plan", "--plain"}, 2, "need the run screen"},
		{"nothing to resume", []string{"--resume"}, 1, "--resume: no open control issue"},
		{"unknown milestone", []string{"--run", "M9"}, 1, `--run: ` + sandbox + ` has no open milestone titled "M9"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox)
			d, _, stderr := testDeps(t, dir, nil, "")
			saveSettings(t, d, sandbox)
			d.runUI = func(context.Context, app.Config, io.Writer) (int, bool) {
				t.Error("the run screen opened")
				return 0, false
			}
			if code := run(context.Background(), append([]string{"--repo", sandbox}, tt.args...), d); code != tt.wantCode {
				t.Fatalf("exit %d, want %d; stderr %s", code, tt.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

// TestRunFlagUsesTheExistingControlIssue: --run on a milestone that has a
// control issue labels it state:dev-ready, keeps its body, and runs it.
func TestRunFlagUsesTheExistingControlIssue(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, _, stderr := testDeps(t, dir, nil, "")
	saveSettings(t, d, sandbox)
	fake := d.gh.(*gh.Fake)
	fake.Issues[0].Body = "hand-written"
	agent := &fakeAgent{gh: fake, label: gh.StateDone}
	d.runAgents = func(string, string, agents.Team, io.Writer) loop.Agents { return agent }
	if code := run(context.Background(), []string{"--repo", sandbox, "--run", "Sandbox M1"}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if len(agent.runs) != 1 || agent.runs[0] != session.Dev {
		t.Errorf("runs = %v, want the Dev", agent.runs)
	}
	if fake.Issues[0].Body != "hand-written" || len(fake.Issues) != 1 {
		t.Errorf("issues = %+v", fake.Issues)
	}
}
