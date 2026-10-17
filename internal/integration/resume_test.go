// Package integration exercises opt-in v4 lifecycle flows on the sandbox only.
package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

const sandbox = "sohaibseniorswe-sys/huddle-sandbox"

// TestSandboxResume runs the real binary and GitHub control loop against a
// throwaway sandbox milestone. Agent processes are scripted so this verifies
// orchestration, not model quality, and never starts autonomous work in the repo.
func TestSandboxResume(t *testing.T) {
	if os.Getenv("HUDDLE_V4_SANDBOX") != "1" {
		t.Skip("set HUDDLE_V4_SANDBOX=1; requires sandbox access and built ./bin/huddle-v4")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	binary, err := filepath.Abs("../../bin/huddle-v4")
	if err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "https://github.com/"+sandbox+".git", checkout)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sandbox clone: %s %v", out, err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(checkout, name), []byte("Sandbox orchestration fixture only.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	client := gh.CLI{}
	if _, err := checks.EnsureLabels(ctx, client, sandbox); err != nil {
		t.Fatal(err)
	}
	title := "v4 resume evidence " + time.Now().UTC().Format("20060102-150405")
	milestone, err := client.CreateMilestone(ctx, sandbox, title, "Throwaway v4 live-loop resume fixture; scripted agents.")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		out, err := exec.Command("gh", "api", "-X", "PATCH", fmt.Sprintf("repos/%s/milestones/%d", sandbox, milestone), "-f", "state=closed").CombinedOutput()
		if err != nil {
			t.Errorf("cleanup milestone: %s %v", out, err)
		}
	})
	control, err := client.CreateIssue(ctx, sandbox, gh.NewIssue{Title: title + " control", Milestone: title, Body: "<!-- huddle:control -->\nScripted resume evidence.", Labels: []string{"control", gh.StatePMWorking}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		out, err := exec.Command("gh", "issue", "close", fmt.Sprint(control), "--repo", sandbox).CombinedOutput()
		if err != nil {
			t.Errorf("cleanup control: %s %v", out, err)
		}
	})
	root := t.TempDir()
	cfg := config.Default(sandbox)
	cfg.Dir = checkout
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	s := store.Store{Root: root}
	p, ok, err := s.FindRepo(sandbox)
	if err != nil || !ok {
		t.Fatal(err)
	}
	p.Phase = "published"
	p.Status = "PLANNED"
	p.Control = control // deliberately stale cache
	if err := s.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	bins := t.TempDir()
	argsFile := filepath.Join(root, "agent-args")
	// The real process runner invokes these using the actual Claude/Codex args.
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
 *--version*) echo '2.1.299'; exit 0;;
 'login status') echo 'Logged in using ChatGPT' >&2; exit 0;;
 'auth status --json') echo '{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}'; exit 0;;
esac
printf '%%s\n' "$*" >> '%s'
cat >/dev/null
gh issue edit %d --repo %s --add-label state:dev-ready >/dev/null || exit 1
echo '{"type":"result","subtype":"success","session_id":"v4-resume-pm","result":"Resumed the PM turn from durable sandbox state."}'
`, argsFile, control, sandbox)
	for _, name := range []string{"codex", "claude"} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	cmd = exec.CommandContext(ctx, binary, "--home", root, "--project", p.ID, "--plain", "--once", "--yes")
	cmd.Env = append(os.Environ(), "HUDDLE_HOME="+root, "PATH="+bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real sandbox resume: %s %v", out, err)
	}
	if !strings.Contains(string(out), "PM (Claude Code), fresh session") || !strings.Contains(string(out), "state:dev-ready") {
		t.Fatalf("wrong resumed turn: %s", out)
	}
	fresh, err := client.IssueView(ctx, sandbox, control)
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Labels) == 0 {
		t.Fatal("missing state after resume")
	}
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("REAL sandbox live-loop evidence (scripted PM): https://github.com/%s/issues/%d\n%s\nAgent args: %s", sandbox, control, out, data)
}
