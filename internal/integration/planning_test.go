package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// TestSandboxPlanningTranscript runs a real read-only companion on an isolated
// sandbox checkout. It intentionally produces no GitHub writes or project edits.
func TestSandboxPlanningTranscript(t *testing.T) {
	if os.Getenv("HUDDLE_V4_REAL_COMPANION") != "1" {
		t.Skip("set HUDDLE_V4_REAL_COMPANION=1 to use subscription quota on sandbox")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	checkout := t.TempDir()
	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "https://github.com/"+sandbox+".git", checkout)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sandbox clone: %s %v", out, err)
	}
	root := t.TempDir()
	s, err := store.Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "planning-evidence", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: "planning-evidence"}
	fixture, err := os.ReadFile("../planning/testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	a := companion.Agent{Choice: agents.Choice{CLI: agents.Claude()}, Instructions: planning.Instructions(""), Dir: checkout, Home: root, Sessions: session.NewCompanion(root, w.ProjectID, 0), Run: runner.Run, Now: time.Now}
	message := "This is a sandbox planning acceptance fixture. All product requirements are in this complete sample; no research or clarification is needed. Return this complete package as a <draft> JSON block, retaining its sections. Do not write files or GitHub.\n" + string(fixture)
	if err := w.History().Append(companion.Entry{Time: time.Now(), Who: companion.WhoYou, Text: message}); err != nil {
		t.Fatal(err)
	}
	reply, err := a.Ask(ctx, "Planning only; no build running.", message)
	if err != nil {
		t.Fatal(err)
	}
	parsed := companion.Parse(reply)
	if parsed.Draft == "" {
		t.Fatalf("no package in real reply: %s", reply)
	}
	d, err := planning.Parse(parsed.Draft)
	if err != nil {
		t.Fatal(err)
	}
	if problems := d.Check(); len(problems) != 0 {
		t.Fatalf("real package invalid: %+v", problems)
	}
	if err := w.History().Append(companion.Entry{Time: time.Now(), Who: companion.WhoCompanion, Text: reply}); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := w.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	resumed := planning.Workspace{Store: s, ProjectID: w.ProjectID}
	context, err := resumed.Context()
	if err != nil || !strings.Contains(context, d.Slug) {
		t.Fatal("draft did not resume")
	}
	status, err := exec.CommandContext(ctx, "git", "-C", checkout, "status", "--porcelain").CombinedOutput()
	if err != nil || len(status) != 0 {
		t.Fatalf("companion edited checkout: %s %v", status, err)
	}
	files, err := filepath.Glob(filepath.Join(root, "logs", "companion", "*.log"))
	if err != nil || len(files) == 0 {
		t.Fatal("missing real process log")
	}
	log, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(log), "--setting-sources user") {
		t.Fatalf("read-only args missing: %v", err)
	}
	t.Logf("REAL sandbox planning transcript (Claude read-only; isolated root and checkout):\n%s\nResumed package %s; %d issues; checkout unchanged; --setting-sources user verified.", reply, d.Slug, len(d.Issues))
}
