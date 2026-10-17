package onboarding

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestAdoptRemoteIdentityAndDirectoryFallback(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet", dir}, {"-C", dir, "remote", "add", "origin", "https://github.com/owner/repo.git"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git fixture %s %v", out, err)
		}
	}
	f := &gh.Fake{Issues: []gh.Issue{{Number: 10, Title: "Existing requirement", Body: "Old acceptance criteria"}}}
	p, err := Adopt(ctx, s, f, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Adopt(ctx, s, f, dir, true)
	if err != nil || again.ID != p.ID || p.Repo != "owner/repo" {
		t.Fatalf("duplicate identity %+v %v", again, err)
	}
	w := planning.Workspace{Store: s, ProjectID: p.ID}
	text, err := w.Context()
	if err != nil {
		t.Fatal(err)
	}
	if text == "" {
		t.Fatal("no adoption input")
	}
	plain := t.TempDir()
	fallback, err := Adopt(ctx, s, f, plain, false)
	if err != nil || fallback.Repo != "" || fallback.Dir != plain {
		t.Fatalf("directory identity %+v %v", fallback, err)
	}
	ps, err := s.List()
	if err != nil || len(ps) != 2 {
		t.Fatalf("registry %+v %v", ps, err)
	}
	if len(f.Log) != 0 || len(f.Created) != 0 {
		t.Fatal("adoption wrote GitHub")
	}
}

func TestPlanNextPreservesRuntimeAndResumesUnpublishedDraft(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	p := store.Project{ID: "next", Repo: "owner/repo", Branch: "v4", Phase: "planning"}
	if err := s.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: p.ID}
	data, err := os.ReadFile("../planning/testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := planning.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	f := &gh.Fake{}
	preview, err := publish.Review(w)
	if err != nil {
		t.Fatal(err)
	}
	result, err := publish.Publish(ctx, f, w, publish.Approval{Confirmed: true, Revision: preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	p, _ = s.Read(p.ID)
	p.Phase = "building"
	p.Status = "ACTIVE"
	p.Started = true
	if err := s.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	fresh, err := BeginNext(ctx, w)
	if err != nil || !fresh {
		t.Fatalf("begin next %v %v", fresh, err)
	}
	if _, exists, err := w.Load(); err != nil || exists {
		t.Fatal("published draft not archived")
	}
	archives, err := filepath.Glob(filepath.Join(s.Data(), "drafts", "archive-next-*"))
	if err != nil || len(archives) != 1 {
		t.Fatal("archive missing")
	}
	runtime, err := s.Read(p.ID)
	if err != nil || runtime.Control != result.Control || runtime.Phase != "building" {
		t.Fatal("planning changed runtime")
	}
	d.Title = "N+1"
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	fresh, err = BeginNext(ctx, w)
	if err != nil || fresh {
		t.Fatal("repeated plan-next discarded unfinished work")
	}
	got, exists, err := w.Load()
	if err != nil || !exists || got.Title != "N+1" {
		t.Fatal("unfinished next draft lost")
	}
}
