package publish

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func fixture(t *testing.T) (planning.Workspace, *gh.Fake, Approval) {
	t.Helper()
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, store.Project{ID: "publish-fixture", Repo: "owner/repo", Branch: "v4", Phase: "planning"}); err != nil {
		t.Fatal(err)
	}
	w := planning.Workspace{Store: s, ProjectID: "publish-fixture"}
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
	p, err := Review(w)
	if err != nil {
		t.Fatal(err)
	}
	return w, &gh.Fake{}, Approval{true, p.Revision}
}

func TestApprovalAndGateBeforeWrites(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(planning.Workspace, *gh.Fake, *Approval)
	}{
		{name: "no approval", change: func(_ planning.Workspace, _ *gh.Fake, a *Approval) { a.Confirmed = false }},
		{name: "stale revision", change: func(_ planning.Workspace, _ *gh.Fake, a *Approval) { a.Revision = "old" }},
		{name: "marker-only active control", change: func(_ planning.Workspace, f *gh.Fake, _ *Approval) {
			f.Issues = []gh.Issue{{Number: 99, Body: gh.ControlMarker, Labels: []string{gh.StateDevReady}}}
		}},
		{name: "GitHub unavailable", change: func(_ planning.Workspace, f *gh.Fake, _ *Approval) { f.ListErr = errors.New("offline") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w, f, a := fixture(t)
			tt.change(w, f, &a)
			if _, err := Publish(context.Background(), f, w, a); err == nil {
				t.Fatal("unexpected publish success")
			}
			if len(f.Log) != 0 || len(f.Written) != 0 || len(f.Created) != 0 {
				t.Fatalf("wrote before approval/gate: %+v", f.Log)
			}
		})
	}
}

func TestFullPublishAndAdoption(t *testing.T) {
	w, f, _ := fixture(t)
	d, _, _ := w.Load()
	second := d.Issues[0]
	second.ID = "02-followup"
	second.Title = "Followup"
	second.DependsOn = []string{d.Issues[0].ID}
	second.Supersedes = []int{50}
	d.Issues = append([]planning.Issue{second}, d.Issues...)
	if err := w.Save(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	preview, err := Review(w)
	if err != nil {
		t.Fatal(err)
	}
	f.Issues = []gh.Issue{{Number: 50, Title: "Old issue"}}
	result, err := Publish(context.Background(), f, w, Approval{true, preview.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if result.Control == 0 || len(result.Issues) != 2 || result.Issues["01-response"] >= result.Issues["02-followup"] {
		t.Fatalf("result %+v", result)
	}
	if len(f.Written) != 3 || len(f.Created) != 7 {
		t.Fatalf("specs/labels: %d %v", len(f.Written), f.Created)
	}
	var control, dependent gh.Issue
	for _, is := range f.Issues {
		if is.Number == result.Control {
			control = is
		}
		if is.Number == result.Issues["02-followup"] {
			dependent = is
		}
	}
	if !strings.HasPrefix(control.Body, gh.ControlMarker+"\n") || strings.Count(control.Body, gh.ControlMarker) != 1 {
		t.Fatal("corrupt control marker")
	}
	if !strings.Contains(dependent.Body, "Depends on: #") || strings.Contains(dependent.Body, "## Requirements") {
		t.Fatalf("issue body not short reference: %s", dependent.Body)
	}
	old, _ := f.IssueView(context.Background(), "owner/repo", 50)
	if !old.Closed || len(f.IssueComments[50]) != 1 || !strings.Contains(f.IssueComments[50][0].Body, "https://github.com/owner/repo/issues/") {
		t.Fatal("missing adoption replacement link")
	}
	cached, err := w.Store.Read(w.ProjectID)
	if err != nil || cached.Phase != "published" || cached.Control != result.Control {
		t.Fatalf("cache %+v %v", cached, err)
	}
	before := len(f.Log)
	again, err := Publish(context.Background(), f, w, Approval{true, preview.Revision})
	if err != nil || again.Control != result.Control || len(f.Log) != before {
		t.Fatalf("retry duplicated work: %+v %v", again, err)
	}
}

func TestPartialPublishRetry(t *testing.T) {
	w, f, a := fixture(t)
	f.CreateErrAt = 2
	if _, err := Publish(context.Background(), f, w, a); err == nil {
		t.Fatal("expected issue-create failure")
	}
	if len(f.Milestones) != 1 {
		t.Fatal("missing journaled milestone")
	}
	f.CreateErrAt = 0
	result, err := Publish(context.Background(), f, w, a)
	if err != nil || result.Control == 0 || len(f.Milestones) != 1 || len(f.Written) != 2 {
		t.Fatalf("retry %+v %v; milestones %d specs %d", result, err, len(f.Milestones), len(f.Written))
	}
}

func TestChangedDraftRequiresNewApproval(t *testing.T) {
	w, f, a := fixture(t)
	d, _, _ := w.Load()
	d.Title = "Changed after review"
	if err := w.Save(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := Publish(context.Background(), f, w, a); err == nil {
		t.Fatal("accepted stale review")
	}
	if len(f.Log) != 0 || len(f.Created) != 0 {
		t.Fatal("stale approval wrote remote state")
	}
}
