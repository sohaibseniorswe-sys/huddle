package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/app"
	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
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
		gh: &gh.Fake{Issues: []gh.Issue{{
			Number: 3, Title: "Sandbox Control", Milestone: "Sandbox M1",
			Labels: []string{"control"},
		}}},
		agents: checks.AgentCLIs{
			LookPath: func(string) (string, error) { return "/bin/true", nil },
			Run: func(_ context.Context, name string, _ ...string) (string, string, error) {
				if name == "codex" {
					return "", "Logged in using ChatGPT\n", nil
				}
				return `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}`, "", nil
			},
		},
		runAgents: func(string, string, io.Writer) loop.Agents { return noAgents{t} },
		clock:     stopClock{},
		// The tests never open the full-screen UI: this stand-in runs the
		// loop with plain output instead.
		runUI: func(ctx context.Context, _ app.Config, l *loop.Loop, _ io.Writer) int {
			l.Report = loop.PlainReporter{Out: &stdout}
			l.Controls = &loop.Controls{}
			return l.Run(ctx)
		},
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
	// No state label: the loop prints the hint and polls; stopClock ends it.
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != loop.ExitInterrupted {
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
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("output %q missing 'aborted'", stdout.String())
	}
}

func TestStartupYesOnceRunsOneTurn(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, stderr := testDeps(t, dir, nil, "")
	fake := d.gh.(*gh.Fake)
	fake.Issues[0].Labels = append(fake.Issues[0].Labels, gh.StateDevReady)
	agent := &fakeAgent{gh: fake, label: gh.StatePMReady}
	var gotDir, gotHome string
	d.runAgents = func(dir, home string, _ io.Writer) loop.Agents { gotDir, gotHome = dir, home; return agent }
	if code := run(context.Background(), []string{"--repo", sandbox, "--yes", "--once"}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "Continue?") {
		t.Error("--yes still prompted")
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
		{[]string{"--repo", sandbox, "--yes"}, true},
		{[]string{"--repo", sandbox, "--yes", "--plain"}, false},
	} {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox)
			d, stdout, _ := testDeps(t, dir, nil, "")
			var stream io.Writer = &bytes.Buffer{} // anything but nil
			d.runAgents = func(_, _ string, s io.Writer) loop.Agents { stream = s; return noAgents{t} }
			var cfg app.Config
			screen := false
			d.runUI = func(_ context.Context, c app.Config, _ *loop.Loop, _ io.Writer) int {
				screen, cfg = true, c
				return 0
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
