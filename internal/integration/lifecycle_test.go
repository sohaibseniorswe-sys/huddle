package integration

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/lanes"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// startSandboxBuild keeps a scripted PM process alive while the integration test
// drafts/checks N+1. The returned function releases the PM and verifies Done.
func startSandboxBuild(t *testing.T, ctx context.Context, s store.Store, id string, r publish.Result, slug string) func() {
	t.Helper()
	p, err := s.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(r.Repo)
	cfg.Dir = p.Dir
	if err := config.SaveID(s.Root, id, cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if err := os.WriteFile(filepath.Join(p.Dir, name), []byte("Sandbox lifecycle fixture. Reload durable state; only operate on this sandbox milestone.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	bins := filepath.Join(s.Root, "build-bin")
	if err := os.MkdirAll(bins, 0700); err != nil {
		t.Fatal(err)
	}
	started, release := filepath.Join(s.Root, "lifecycle-pm-started"), filepath.Join(s.Root, "lifecycle-release-pm")
	argsPath := filepath.Join(s.Root, "lifecycle-agent-args")
	var work int
	for _, n := range r.Issues {
		work = n
		break
	}
	script := fmt.Sprintf(`#!/bin/sh
case "$*" in
 *--version*) echo '2.1.299'; exit 0;;
 'login status') echo 'Logged in using ChatGPT' >&2; exit 0;;
 'auth status --json') echo '{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty"}'; exit 0;;
esac
printf '%%s\n' "$*" >> '%s'
cat >/dev/null
gh issue view %d --repo %s --json body,comments,labels,state >/dev/null || exit 1
cat AGENTS.md CLAUDE.md Session.md >/dev/null || exit 1
git log -1 >/dev/null || exit 1
case "${0##*/}" in
codex)
 git pull --ff-only >/dev/null || exit 1
 test -f 'docs/%s/README.md' || exit 1
 gh issue close %d --repo %s >/dev/null || exit 1
 gh issue edit %d --repo %s --add-label state:pm-ready >/dev/null || exit 1
 printf 'session id: lifecycle-dev\n' >&2
 echo READY
 ;;
claude)
 touch '%s'
 while ! test -f '%s'; do sleep 0.05; done
 gh issue comment %d --repo %s --body 'Status: Completed. Sandbox fixture issue verified; lifecycle complete.' >/dev/null || exit 1
 gh issue edit %d --repo %s --add-label state:done >/dev/null || exit 1
 gh issue close %d --repo %s >/dev/null || exit 1
 echo '{"type":"result","subtype":"success","session_id":"lifecycle-pm","result":"Sandbox lifecycle completed."}'
 ;;
esac
`, argsPath, r.Control, r.Repo, slug, work, r.Repo, r.Control, r.Repo, started, release, r.Control, r.Repo, r.Control, r.Repo, r.Control, r.Repo)
	for _, name := range []string{"codex", "claude"} {
		if err := os.WriteFile(filepath.Join(bins, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	binary, err := filepath.Abs("../../bin/huddle-v4")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, binary, "--home", s.Root, "--project", id, "--plain", "--yes")
	cmd.Env = append(os.Environ(), "HUDDLE_HOME="+s.Root, "PATH="+bins+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 2 * time.Second
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := false
	wait := func() {
		if done {
			return
		}
		done = true
		if err := os.WriteFile(release, nil, 0600); err != nil {
			t.Error(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("real build loop: %s %v", output.String(), err)
		}
		if !strings.Contains(output.String(), "state:done: the milestone is finished") || !strings.Contains(output.String(), "Dev (Codex), fresh session") || !strings.Contains(output.String(), "PM (Claude Code), fresh session") {
			t.Fatalf("lifecycle did not finish: %s", output.String())
		}
		args, err := os.ReadFile(argsPath)
		if err != nil || strings.Contains(string(args), "resume") {
			t.Fatalf("non-fresh args: %s %v", args, err)
		}
		t.Logf("REAL sandbox binary build loop (scripted Dev/PM):\n%s\nAgent args:\n%s", output.String(), args)
	}
	t.Cleanup(wait)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("PM process did not start")
		case <-ticker.C:
		}
	}
	live, active, err := lanes.Active(s.Root)
	if err != nil || !active || live.CLI != "claude" {
		t.Fatalf("missing real PM lane %+v %v %v", live, active, err)
	}
	return wait
}
