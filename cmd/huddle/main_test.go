package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

const sandbox = "sohaibseniorswe-sys/huddle-sandbox"

// testDeps returns deps whose external calls all succeed, rooted at dir.
func testDeps(dir string, env map[string]string, stdin string) (deps, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	return deps{
		stdin:     strings.NewReader(stdin),
		stdout:    &stdout,
		stderr:    &stderr,
		getwd:     func() (string, error) { return dir, nil },
		lookupEnv: func(k string) (string, bool) { v, ok := env[k]; return v, ok },
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
	d, stdout, _ := testDeps(t.TempDir(), nil, "")
	if code := run(context.Background(), []string{"--version"}, d); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(stdout.String(), "huddle ") {
		t.Errorf("--version printed %q", stdout.String())
	}
}

func TestRepoFlagValidation(t *testing.T) {
	for _, bad := range []string{"palantir", "a/b/c", "https://github.com/a/b", "", "owner/"} {
		d, _, stderr := testDeps(t.TempDir(), nil, "")
		if code := run(context.Background(), []string{"--repo", bad}, d); code != 2 {
			t.Errorf("--repo %q: exit %d, want 2", bad, code)
		}
		if !strings.Contains(stderr.String(), "--repo") {
			t.Errorf("--repo %q: stderr %q does not mention --repo", bad, stderr.String())
		}
	}
}

func TestUnexpectedArgs(t *testing.T) {
	d, _, _ := testDeps(t.TempDir(), nil, "")
	if code := run(context.Background(), []string{"extra"}, d); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

func TestRefusesWithAPIKeys(t *testing.T) {
	for _, key := range checks.APIKeyVars {
		t.Run(key, func(t *testing.T) {
			dir := gitRepo(t, "https://github.com/"+sandbox+".git")
			d, stdout, stderr := testDeps(dir, map[string]string{key: "sk-test"}, "y\n")
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
	d, stdout, stderr := testDeps(dir, map[string]string{}, "\n")
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"target: " + sandbox + " (override)",
		"working tree clean",
		"created empty Session.md",
		"labels ready (created control, state:dev-ready",
		`control issue #3 "Sandbox Control", milestone "Sandbox M1", state: none`,
		"Continue? [Y/n]",
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
	d, stdout, _ := testDeps(dir, nil, "n\n")
	if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "aborted") {
		t.Errorf("output %q missing 'aborted'", stdout.String())
	}
}

func TestStartupYesSkipsPrompt(t *testing.T) {
	dir := gitRepo(t, "https://github.com/"+sandbox)
	d, stdout, _ := testDeps(dir, nil, "")
	if code := run(context.Background(), []string{"--repo", sandbox, "--yes", "--once"}, d); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stdout.String(), "Continue?") {
		t.Error("--yes still prompted")
	}
}

func TestStartupFailures(t *testing.T) {
	t.Run("default target, sandbox clone", func(t *testing.T) {
		dir := gitRepo(t, "https://github.com/"+sandbox)
		d, _, stderr := testDeps(dir, nil, "y\n")
		if code := run(context.Background(), nil, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), checks.DefaultRepo) {
			t.Errorf("stderr %q", stderr.String())
		}
	})
	t.Run("dirty tree", func(t *testing.T) {
		dir := gitRepo(t, "https://github.com/"+sandbox)
		os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("x"), 0o644)
		d, _, stderr := testDeps(dir, nil, "y\n")
		if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "scratch.txt") {
			t.Errorf("stderr %q", stderr.String())
		}
	})
	t.Run("not a git repo", func(t *testing.T) {
		d, _, stderr := testDeps(t.TempDir(), nil, "y\n")
		if code := run(context.Background(), []string{"--repo", sandbox}, d); code != 1 {
			t.Fatalf("exit %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "not inside a git repo") {
			t.Errorf("stderr %q", stderr.String())
		}
	})
	t.Run("gh not logged in", func(t *testing.T) {
		dir := gitRepo(t, "https://github.com/"+sandbox)
		d, _, stderr := testDeps(dir, nil, "y\n")
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
