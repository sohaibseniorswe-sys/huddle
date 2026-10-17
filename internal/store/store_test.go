package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestRegistryAndIsolation(t *testing.T) {
	ctx := context.Background()
	a, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []Project{
		{ID: "id-only", Phase: "planning"},
		{ID: "with-repo", Name: "Example", Dir: "/example", Repo: "owner/repo", Phase: "building"},
	} {
		if err := a.Put(ctx, p); err != nil {
			t.Fatal(err)
		}
		got, err := a.Read(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		got.Touched = p.Touched
		if !reflect.DeepEqual(got, p) {
			t.Fatalf("round trip: %#v != %#v", got, p)
		}
	}
	ps, err := a.List()
	if err != nil || len(ps) != 2 {
		t.Fatalf("list: %v %v", ps, err)
	}
	ps, err = b.List()
	if err != nil || len(ps) != 0 {
		t.Fatalf("roots share state: %v %v", ps, err)
	}
	for _, id := range []string{"../escape", "", "a/b"} {
		if err := a.Put(ctx, Project{ID: id, Phase: "planning"}); err == nil {
			t.Fatalf("accepted id %q", id)
		}
	}
	if err := a.Put(ctx, Project{ID: "valid", Phase: "unknown"}); err == nil {
		t.Fatal("accepted unknown phase")
	}
	if err := a.Write("../escape", nil); err == nil {
		t.Fatal("accepted traversal")
	}
}

func TestMigration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	old := filepath.Join(root, "projects", "owner_repo.yaml")
	data := []byte("# exact comments retained\nrepo: owner/repo\ndir: /old/path\nautonomy: 6\nagents:\n  dev:\n    cli: codex\n")
	if err := os.MkdirAll(filepath.Dir(old), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{s.ConfigPath("owner/repo"), filepath.Join(root, "migration-backup", "owner_repo.yaml"), old} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != string(data) {
			t.Fatalf("lossy migration %s: %s %v", path, got, err)
		}
	}
	before, err := s.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.git(ctx, "rev-parse", "HEAD")
	if err != nil || string(before) != string(after) {
		t.Fatal("migration not idempotent")
	}
	p, ok, err := s.FindRepo("owner/repo")
	if err != nil || !ok || p.Dir != "/old/path" {
		t.Fatalf("registry: %v %v", p, err)
	}
}

func TestClonePortableStore(t *testing.T) {
	ctx := context.Background()
	a, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Put(ctx, Project{ID: "portable", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, t.TempDir(), a.Data())
	if err != nil {
		t.Fatal(err)
	}
	ps, err := b.List()
	if err != nil || len(ps) != 1 || ps[0].ID != "portable" {
		t.Fatalf("clone: %v %v", ps, err)
	}
	if _, err := Open(ctx, t.TempDir(), "/nonexistent/huddle-data"); err == nil {
		t.Fatal("clone failure hidden")
	}
}

func TestRegistryCommitsDoNotCheckpointDrafts(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Write("drafts/plan/pending.md", []byte("still drafting")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, Project{ID: "plan", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	out, err := s.git(ctx, "ls-files", "drafts")
	if err != nil || len(out) != 0 {
		t.Fatalf("registry commit checkpointed draft: %s %v", out, err)
	}
	if err := s.Checkpoint(ctx, "Agreed checkpoint"); err != nil {
		t.Fatal(err)
	}
	out, err = s.git(ctx, "ls-files", "drafts")
	if err != nil || len(out) == 0 {
		t.Fatalf("checkpoint missed draft: %s %v", out, err)
	}
}

func TestConcurrentRegistryCommits(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- s.Put(ctx, Project{ID: fmt.Sprintf("project-%d", i), Phase: "planning"})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ps, err := s.List()
	if err != nil || len(ps) != 4 {
		t.Fatalf("concurrent records: %+v %v", ps, err)
	}
	b, err := os.ReadFile(filepath.Join(s.Data(), "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if !strings.Contains(string(b), p.ID) {
			t.Fatalf("index lost %s: %s", p.ID, b)
		}
	}
}
