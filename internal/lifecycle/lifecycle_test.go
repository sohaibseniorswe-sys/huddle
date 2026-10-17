package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func TestDerive(t *testing.T) {
	for _, tt := range []struct {
		name, label                                 string
		started, closed, milestoneClosed, noControl bool
		phase                                       Phase
		status                                      string
		bad                                         bool
	}{
		{name: "draft only", noControl: true, phase: Planning, status: "PLANNED"},
		{name: "published", label: gh.StateDevReady, phase: Published, status: "PLANNED"},
		{name: "next dev turn", label: gh.StateDevReady, started: true, phase: Building, status: "ACTIVE"},
		{name: "dev working", label: gh.StateDevWorking, phase: Building, status: "ACTIVE"},
		{name: "pm ready", label: gh.StatePMReady, phase: Building, status: "ACTIVE"},
		{name: "pm working", label: gh.StatePMWorking, phase: Building, status: "ACTIVE"},
		{name: "paused", label: gh.StateHumanRequired, phase: Building, status: "ACTIVE"},
		{name: "done open control", label: gh.StateDone, phase: Done, status: "DONE"},
		{name: "done closed control", label: gh.StateDone, closed: true, phase: Done, status: "DONE"},
		{name: "closed milestone", label: gh.StateDevReady, milestoneClosed: true, phase: Done, status: "DONE"},
		{name: "closed without done", closed: true, phase: Building, status: "ACTIVE"},
		{name: "unknown", label: "state:unknown", bad: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Derive(Snapshot{HasControl: !tt.noControl, Started: tt.started, MilestoneClosed: tt.milestoneClosed, Control: gh.Issue{Number: 1, Labels: []string{tt.label}, Closed: tt.closed}})
			if (err != nil) != tt.bad || (!tt.bad && (got.Phase != tt.phase || got.Status != tt.status)) {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

func TestRefreshOverridesCache(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	p := store.Project{ID: "stable", Repo: "owner/repo", Phase: "done", Status: "DONE"}
	if err := s.Put(ctx, p); err != nil {
		t.Fatal(err)
	}
	client := &gh.Fake{Issues: []gh.Issue{{Number: 9, Milestone: "M", Labels: []string{"control", gh.StatePMReady}}}, Milestones: []gh.Milestone{{Title: "M"}}}
	got, err := Refresh(ctx, client, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID || got.Phase != "building" || got.Status != "ACTIVE" || got.Control != 9 {
		t.Fatalf("refresh %+v", got)
	}
	persisted, err := s.Read(p.ID)
	if err != nil || persisted.Phase != got.Phase {
		t.Fatalf("cache %+v %v", persisted, err)
	}
	client.ListErr = errors.New("offline")
	if _, err := Refresh(ctx, client, s, got); err == nil {
		t.Fatal("error hidden")
	}
	persisted, _ = s.Read(p.ID)
	if persisted.Phase != "building" {
		t.Fatal("failed fetch changed cache")
	}
}

func TestPlanningIdentityNeedsNoRepo(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Refresh(ctx, &gh.Fake{ListErr: errors.New("must not call")}, s, store.Project{ID: "only-id", Phase: "planning"})
	if err != nil || p.Phase != "planning" || p.Repo != "" || p.Dir != "" {
		t.Fatalf("%+v %v", p, err)
	}
}

func TestReadStartEvidenceAndDrift(t *testing.T) {
	for _, tt := range []struct {
		name     string
		comments []gh.Comment
		cached   bool
		want     Phase
	}{
		{name: "never started", want: Published},
		{name: "cached started", cached: true, want: Building},
		{name: "agent handoff", comments: []gh.Comment{{Body: "Status: Progress"}}, want: Building},
		{name: "planning comment", comments: []gh.Comment{{Body: "Please build"}}, want: Published},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := &gh.Fake{Issues: []gh.Issue{{Number: 1, Milestone: "M", Labels: []string{"control", gh.StateDevReady}}}, Milestones: []gh.Milestone{{Title: "M"}}, IssueComments: map[int][]gh.Comment{1: tt.comments}}
			snap, err := Read(context.Background(), client, store.Project{Repo: "o/r", Control: 1, Started: tt.cached})
			if err != nil {
				t.Fatal(err)
			}
			got, err := Derive(snap)
			if err != nil || got.Phase != tt.want {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
