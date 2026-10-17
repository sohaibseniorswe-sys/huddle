package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/lifecycle"
	"github.com/sohaibseniorswe-sys/huddle/internal/onboarding"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// This opt-in test is limited to the ONE repository authorized by the PM.
// The explicit durable root preserves partial setup for inspection/recovery.
func TestNewProjectLifecycle(t *testing.T) {
	repo, root := os.Getenv("HUDDLE_V4_NEW_REPO"), os.Getenv("HUDDLE_V4_NEW_HOME")
	if repo == "" {
		t.Skip("set HUDDLE_V4_NEW_REPO and HUDDLE_V4_NEW_HOME for the authorized single-repo acceptance run")
	}
	if !regexp.MustCompile(`^sohaibseniorswe-sys/huddle-sandbox-new-[0-9]{8}-[0-9]{4}$`).MatchString(repo) || !filepath.IsAbs(root) || root == os.Getenv("HOME")+"/.huddle" {
		t.Fatal("acceptance requires an authorized name and isolated absolute root")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "gh", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("gh %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	client := gh.CLI{}
	s, err := store.Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	id := "new-project-acceptance"
	p, err := s.Read(id)
	if errors.Is(err, os.ErrNotExist) {
		p = store.Project{ID: id, Phase: "planning", Status: "PLANNED"}
		if err := s.Put(ctx, p); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: id}
	d, exists, err := w.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		b, err := os.ReadFile("../planning/testdata/good.json")
		if err != nil {
			t.Fatal(err)
		}
		d, err = planning.Parse(string(b))
		if err != nil {
			t.Fatal(err)
		}
		d.Title = "New project acceptance " + filepath.Base(root)
		d.Project = &planning.NewProject{Name: "New project acceptance", Dir: filepath.Join(root, "workspace"), Repo: repo, Branch: "v4-acceptance"}
		if err := w.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
		if err := w.Checkpoint(ctx); err != nil {
			t.Fatal(err)
		}
		// Saving a proposal has no directory or remote side effects.
		if _, err := os.Stat(d.Project.Dir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("planning created directory")
		}
	}
	// Reload committed identity/draft as a second session before approving.
	resumed, err := store.Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	w.Store = resumed
	p, err = resumed.Read(id)
	if err != nil || p.ID != id {
		t.Fatalf("lost identity %+v %v", p, err)
	}
	preview, err := publish.Review(w)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("REAL mid-planning resume: id=%s phase=%s name=%s dir=%s repo=%s; proposed destination=%+v", id, p.Phase, p.Name, p.Dir, p.Repo, *preview.Draft.Project)
	// Archive even if later acceptance fails; evidence remains readable and private.
	t.Cleanup(func() {
		out, err := exec.Command("gh", "repo", "archive", repo, "--yes").CombinedOutput()
		if err != nil {
			t.Errorf("archive %s: %s %v", repo, out, err)
		} else {
			t.Logf("Archived private acceptance repository %s", repo)
		}
	})
	result, err := publish.Publish(ctx, client, w, publish.Approval{Confirmed: true, Revision: preview.Revision})
	t.Logf("REAL first publish result: %+v (error=%v); isolated root=%s", result, err, root)
	if err != nil {
		t.Fatal(err)
	}

	p, err = s.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	p, err = lifecycle.Refresh(ctx, client, s, p)
	if err != nil || p.Phase != "published" {
		t.Fatalf("published %+v %v", p, err)
	}
	cfg, exists, err := config.LoadID(root, id)
	if err != nil || !exists || cfg.Repo != repo || cfg.Dir != p.Dir {
		t.Fatalf("new destination settings %+v %v %v", cfg, exists, err)
	}
	if _, err := checks.EnsureAgentFiles(p.Dir); err != nil {
		t.Fatalf("new checkout cannot start without harness scaffolding: %v", err)
	}
	finish := startSandboxBuild(t, ctx, s, id, result, d.Slug)
	p, err = s.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	p, err = lifecycle.Refresh(ctx, client, s, p)
	if err != nil || p.Phase != "building" {
		t.Fatalf("building %+v %v", p, err)
	}
	if fresh, err := onboarding.BeginNext(ctx, w); err != nil || !fresh {
		t.Fatalf("plan-next while building %v %v", fresh, err)
	}
	d.Project = nil
	d.Title += " next"
	d.Slug += "-next"
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	next, err := publish.Review(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publish.Publish(ctx, client, w, publish.Approval{Confirmed: true, Revision: next.Revision}); err == nil || !strings.Contains(err.Error(), "active control") {
		t.Fatalf("N+1 was not gated: %v", err)
	}
	finish()
	run("api", "-X", "PATCH", fmt.Sprintf("repos/%s/milestones/%d", repo, result.Milestone), "-f", "state=closed")
	p, err = s.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	p, err = lifecycle.Refresh(ctx, client, s, p)
	if err != nil || p.Phase != "done" {
		t.Fatalf("done %+v %v", p, err)
	}
	if fresh, err := onboarding.BeginNext(ctx, w); err != nil || fresh {
		t.Fatalf("lost N+1 draft %v %v", fresh, err)
	}
	nextResult, err := publish.Publish(ctx, client, w, publish.Approval{Confirmed: true, Revision: next.Revision})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("REAL full new→plan→publish→build→done→plan-next: first control https://github.com/%s/issues/%d; next control https://github.com/%s/issues/%d; specs https://github.com/%s/tree/%s/docs/%s", repo, result.Control, repo, nextResult.Control, repo, result.Branch, preview.Draft.Slug)
	if err := s.Put(ctx, store.Project{ID: "second-planning-project", Phase: "planning", Status: "PLANNED"}); err != nil {
		t.Fatal(err)
	}
	binary, err := filepath.Abs("../../bin/huddle-v4")
	if err != nil {
		t.Fatal(err)
	}
	freshRoot := filepath.Join(root, "fresh-machine")
	cmd := exec.CommandContext(ctx, binary, "--home", freshRoot, "--data-remote", s.Data(), "--list")
	cmd.Env = append(os.Environ(), "HUDDLE_HOME="+freshRoot)
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), id) || !strings.Contains(string(out), "second-planning-project") || strings.Count(string(out), "PLANNED") != 2 {
		t.Fatalf("fresh-machine list %s %v", out, err)
	}
	t.Logf("REAL fresh-machine clone/list before cleanup:\n%s", out)
	// Finish fixtures before archival, retaining the complete history.
	for _, n := range nextResult.Issues {
		if err := client.CloseIssue(ctx, repo, n, "Acceptance fixture cleanup."); err != nil {
			t.Fatal(err)
		}
	}
	if err := (gh.Control{Client: client, Repo: repo, Number: nextResult.Control}).SetState(ctx, gh.StateDone); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseIssue(ctx, repo, nextResult.Control, "Acceptance fixture cleanup."); err != nil {
		t.Fatal(err)
	}
	run("api", "-X", "PATCH", fmt.Sprintf("repos/%s/milestones/%d", repo, nextResult.Milestone), "-f", "state=closed")
}
