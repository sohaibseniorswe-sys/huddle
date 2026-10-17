package start

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestStaleDirectoryAndRepair(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	p := store.Project{ID: "stable", Repo: "owner/repo", Dir: filepath.Join(t.TempDir(), "deleted"), Phase: "building"}
	if err := s.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	stale, err := StalePath(p)
	if err != nil || !stale {
		t.Fatalf("%v %v", stale, err)
	}
	if _, err := Repair(ctx, s, p, PathRepair{Dir: p.Dir}); err == nil {
		t.Fatal("accepted missing replacement")
	}
	replacement := t.TempDir()
	p, err = Repair(ctx, s, p, PathRepair{Dir: replacement})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "stable" || p.Repo != "owner/repo" || p.Dir != replacement {
		t.Fatalf("identity changed: %+v", p)
	}
	persisted, err := s.Read(p.ID)
	if err != nil || persisted.Dir != replacement {
		t.Fatalf("repair did not persist: %+v %v", persisted, err)
	}
	p, err = Repair(ctx, s, p, PathRepair{Drop: true})
	if err != nil || !p.Archived {
		t.Fatalf("drop: %+v %v", p, err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	p.Dir = file
	stale, err = StalePath(p)
	if err != nil || !stale {
		t.Fatal("file accepted as directory")
	}
}

func TestAbandonedDraftSurvivesAndPrunes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := store.Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "plan", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Write("drafts/plan/README.md", []byte("unfinished draft")); err != nil {
		t.Fatal(err)
	}
	if err := s.Checkpoint(ctx, "Planning checkpoint"); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	ps, err := reopened.List()
	if err != nil || len(ps) != 1 || ps[0].Phase != "planning" {
		t.Fatalf("abandoned plan lost: %+v %v", ps, err)
	}
	data, err := os.ReadFile(filepath.Join(reopened.Data(), "drafts/plan/README.md"))
	if err != nil || string(data) != "unfinished draft" {
		t.Fatalf("draft lost: %s %v", data, err)
	}
	if err := reopened.PruneDraft(ctx, "plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(reopened.Data(), "drafts/plan")); !os.IsNotExist(err) {
		t.Fatal("draft not pruned")
	}
	if _, err := reopened.Read("plan"); err != nil {
		t.Fatal("prune removed identity")
	}
}
