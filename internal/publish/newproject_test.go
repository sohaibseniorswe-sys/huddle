package publish

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

type newRepoFake struct {
	gh.Fake
	fail  bool
	calls int
}

func (f *newRepoFake) EnsurePrivateRepository(context.Context, string, string, string) error {
	f.calls++
	if f.fail {
		return errors.New("offline")
	}
	return nil
}

func newFixture(t *testing.T) (planning.Workspace, *newRepoFake, Preview) {
	t.Helper()
	w, _, _ := fixture(t)
	ctx := context.Background()
	p := store.Project{ID: w.ProjectID, Phase: "planning", Status: "PLANNED"}
	if err := w.Store.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	d, _, _ := w.Load()
	d.Project = &planning.NewProject{Name: "New project", Dir: filepath.Join(t.TempDir(), "new-project"), Repo: "owner/new-project", Branch: "main"}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if err := w.Checkpoint(ctx); err != nil {
		t.Fatal(err)
	}
	preview, err := Review(w)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := w.Store.Read(w.ProjectID)
	if err != nil || cached.Name != d.Project.Name || cached.Dir != "" || cached.Repo != "" {
		t.Fatalf("planning output cache %+v %v", cached, err)
	}
	return w, &newRepoFake{}, preview
}

func TestNewProjectApprovalAndValidation(t *testing.T) {
	ctx := context.Background()
	w, f, p := newFixture(t)
	if _, err := Publish(ctx, f, w, Approval{}); err == nil {
		t.Fatal("unapproved creation")
	}
	if f.calls != 0 {
		t.Fatal("created repo before approval")
	}
	if _, err := os.Stat(p.Project.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("created directory before approval")
	}
	d, _, _ := w.Load()
	d.Project.Dir = "relative"
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(w); err == nil {
		t.Fatal("accepted relative path")
	}
	d.Project.Dir = p.Project.Dir
	d.Project.Repo = "../bad"
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := Review(w); err == nil {
		t.Fatal("accepted invalid repo")
	}
}

func TestNewProjectResumesCreationAndClone(t *testing.T) {
	ctx := context.Background()
	w, f, preview := newFixture(t)
	a := Approval{true, preview.Revision}
	f.fail = true
	if _, err := Publish(ctx, f, w, a); err == nil {
		t.Fatal("expected network failure")
	}
	p, err := w.Store.Read(w.ProjectID)
	if err != nil || p.ID != w.ProjectID || p.Name != "New project" || p.Dir != preview.Project.Dir || p.Repo != "" || p.Phase != "planning" {
		t.Fatalf("partial outputs %+v %v", p, err)
	}
	// Restart from the durable store/draft; approval still covers the same outputs.
	s, err := store.Open(ctx, w.Store.Root, "")
	if err != nil {
		t.Fatal(err)
	}
	w = planning.Workspace{Store: s, ProjectID: w.ProjectID}
	resumed, err := Review(w)
	if err != nil || resumed.Revision != a.Revision {
		t.Fatalf("resume changed proposal %v", err)
	}
	// A real local Git repository stands in for GitHub only for clone transport.
	source := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", source}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %s %v", out, err)
		}
	}
	git("init", "--initial-branch=main")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@localhost", "commit", "--allow-empty", "-m", "Initial")
	gitConfig := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[url \""+source+"\"]\n insteadOf = https://github.com/owner/new-project.git\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	// Model a killed clone after the durable setup intent: the owned temporary
	// directory is incomplete and must be retried without touching user files.
	if err := os.Mkdir(stagingPath(preview.Project), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingPath(preview.Project), "partial"), []byte("incomplete clone"), 0600); err != nil {
		t.Fatal(err)
	}
	f.fail = false
	result, err := Publish(ctx, f, w, a)
	if err != nil {
		t.Fatal(err)
	}
	if result.Control == 0 {
		t.Fatal("missing milestone control")
	}
	p, err = s.Read(w.ProjectID)
	if err != nil || p.Repo != "owner/new-project" || p.Phase != "published" {
		t.Fatalf("outputs %+v %v", p, err)
	}
	if _, err := os.Stat(filepath.Join(p.Dir, ".git")); err != nil {
		t.Fatal("bootstrap missing", err)
	}
	if _, err := checks.EnsureAgentFiles(p.Dir); err != nil {
		t.Fatalf("new project cannot start: %v", err)
	}
	calls := f.calls
	if _, err := Publish(ctx, f, w, a); err != nil {
		t.Fatal(err)
	}
	if f.calls != calls {
		t.Fatal("completed publish repeated setup")
	}
}

func TestNewProjectRefusesExistingDirectoryAndChangedSetup(t *testing.T) {
	ctx := context.Background()
	t.Run("existing directory", func(t *testing.T) {
		w, f, p := newFixture(t)
		if err := os.Mkdir(p.Project.Dir, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Publish(ctx, f, w, Approval{true, p.Revision}); err == nil {
			t.Fatal("reused existing directory")
		}
		if f.calls != 0 {
			t.Fatal("created repo despite directory conflict")
		}
	})
	t.Run("destination changed after partial setup", func(t *testing.T) {
		w, f, p := newFixture(t)
		f.fail = true
		_, _ = Publish(ctx, f, w, Approval{true, p.Revision})
		d, _, _ := w.Load()
		d.Project.Repo = "owner/different"
		if err := w.Save(ctx, d); err != nil {
			t.Fatal(err)
		}
		next, err := Review(w)
		if err != nil {
			t.Fatal(err)
		}
		calls := f.calls
		if _, err := Publish(ctx, f, w, Approval{true, next.Revision}); err == nil {
			t.Fatal("redirected partial setup")
		}
		if f.calls != calls {
			t.Fatal("created another repo")
		}
	})
}
