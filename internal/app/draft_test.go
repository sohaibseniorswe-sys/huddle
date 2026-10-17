package app

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestCompanionDraftPersistsWithoutGitHubWrites(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "project-Y", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: "project-Y"}
	f.m.cfg.Draft = &w
	f.m.cfg.History = w.History()
	data, err := os.ReadFile("../planning/testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	f.comp.replies = []string{"Draft ready. <draft>" + string(data) + "</draft>"}
	f.keys("Draft the READY fixture", "enter")
	d, exists, err := w.Load()
	if err != nil || !exists || len(d.Check()) != 0 {
		t.Fatalf("draft %+v %v %v", d, exists, err)
	}
	if f.m.view != viewPlan || !strings.Contains(f.m.planText, "Success criteria") {
		t.Fatal("exact package not shown")
	}
	if len(f.gh.Log) != 0 {
		t.Fatalf("planning wrote GitHub: %v", f.gh.Log)
	}
	context, err := w.Context()
	if err != nil || !strings.Contains(context, "Draft the READY fixture") {
		t.Fatalf("conversation lost: %s %v", context, err)
	}
}

func TestPublishCardRequiresApprovalAndDoesNotStartBuild(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "publish", Repo: "owner/repo", Branch: "v4", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: "publish"}
	f.m.cfg.Draft = &w
	f.gh.Issues = nil
	b, err := os.ReadFile("../planning/testdata/good.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := planning.Parse(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Save(ctx, d); err != nil {
		t.Fatal(err)
	}
	f.m.cfg.Begin = func() { t.Error("publishing started a build without a Start choice") }
	f.keys("/publish", "enter")
	if f.m.card.kind != cardPlan || f.m.card.revision == "" {
		t.Fatal("missing exact-draft review card")
	}
	if len(f.gh.Log) != 0 || len(f.gh.Created) != 0 {
		t.Fatal("review wrote GitHub")
	}
	f.m.focus = focusCard
	f.keys("enter")
	p, err := s.Read("publish")
	if err != nil || p.Phase != "published" || p.Control == 0 {
		t.Fatalf("approved publish %+v %v", p, err)
	}
}
