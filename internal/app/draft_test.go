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
