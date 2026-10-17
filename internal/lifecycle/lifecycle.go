// Package lifecycle derives project phase from the project repository. Registry
// phase/status are caches; Refresh re-reads GitHub before routing an open project.
package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// Phase is a project's lifecycle phase.
type Phase string

const (
	Planning  Phase = "planning"
	Published Phase = "published"
	Building  Phase = "building"
	Done      Phase = "done"
)

// Snapshot is authoritative runtime state, without any registry cache fields.
type Snapshot struct {
	HasControl      bool
	MilestoneClosed bool
	Control         gh.Issue
	Started         bool
}

// State is the phase and milestone status used in project listings/routing.
type State struct {
	Phase  Phase
	Status string
}

// Derive computes phase from a GitHub snapshot. dev-ready is published only
// until the first turn; subsequent dev-ready handoffs remain building.
func Derive(s Snapshot) (State, error) {
	if !s.HasControl {
		return State{Planning, "PLANNED"}, nil
	}
	if s.MilestoneClosed || has(s.Control.Labels, gh.StateDone) {
		return State{Done, "DONE"}, nil
	}
	for _, label := range []string{gh.StateHumanRequired, gh.StatePMReady, gh.StatePMWorking, gh.StateDevWorking} {
		if has(s.Control.Labels, label) {
			return State{Building, "ACTIVE"}, nil
		}
	}
	if has(s.Control.Labels, gh.StateDevReady) {
		if s.Started {
			return State{Building, "ACTIVE"}, nil
		}
		return State{Published, "PLANNED"}, nil
	}
	// A closed control without done is drift, not evidence of completion.
	if s.Control.Closed {
		return State{Building, "ACTIVE"}, nil
	}
	return State{}, fmt.Errorf("control #%d has no recognized lifecycle state", s.Control.Number)
}

func has(labels []string, want string) bool {
	return slices.ContainsFunc(labels, func(label string) bool { return strings.EqualFold(label, want) })
}

// Refresh reads runtime truth and commits its refreshed cache. An id-only
// planning project needs no GitHub calls. Failure leaves the cache untouched.
func Refresh(ctx context.Context, client gh.Client, s store.Store, p store.Project) (store.Project, error) {
	if p.Repo == "" {
		p.Phase = string(Planning)
		p.Status = "PLANNED"
		return p, s.Put(ctx, p)
	}
	snap, err := Read(ctx, client, p)
	if err != nil {
		return p, err
	}
	derived, err := Derive(snap)
	if err != nil {
		return p, err
	}
	p.Phase, p.Status = string(derived.Phase), derived.Status
	if snap.HasControl {
		p.Control = snap.Control.Number
		p.Started = snap.Started
	}
	return p, s.Put(ctx, p)
}

// Read discovers the latest active control (or latest completed control) and
// fetches its current labels again. Listing projects never calls this function.
func Read(ctx context.Context, client gh.Client, p store.Project) (Snapshot, error) {
	open, err := client.ListIssues(ctx, p.Repo, "open")
	if err != nil {
		return Snapshot{}, err
	}
	var control gh.Issue
	for _, is := range open {
		if gh.IsControl(is.Labels, is.Body) && !has(is.Labels, gh.StateDone) {
			if control.Number != 0 {
				return Snapshot{}, fmt.Errorf("%s has multiple active control issues: #%d and #%d", p.Repo, control.Number, is.Number)
			}
			control = is
		}
	}
	if control.Number == 0 {
		closed, err := client.ListIssues(ctx, p.Repo, "closed")
		if err != nil {
			return Snapshot{}, err
		}
		for _, is := range append(open, closed...) {
			if gh.IsControl(is.Labels, is.Body) && is.Number > control.Number {
				control = is
			}
		}
	}
	if control.Number == 0 {
		return Snapshot{}, nil
	}
	fresh, err := client.IssueView(ctx, p.Repo, control.Number)
	if err != nil {
		return Snapshot{}, err
	}
	control.Labels, control.Closed = fresh.Labels, fresh.Closed
	snap := Snapshot{HasControl: true, Control: control, Started: p.Started && p.Control == control.Number}
	if control.Milestone != "" {
		milestones, err := client.ListMilestones(ctx, p.Repo)
		if err != nil {
			return Snapshot{}, err
		}
		snap.MilestoneClosed = !slices.ContainsFunc(milestones, func(m gh.Milestone) bool { return m.Title == control.Milestone })
	}
	comments, err := client.Comments(ctx, p.Repo, control.Number)
	if err != nil {
		return Snapshot{}, err
	}
	// Human planning comments can predate the first build. Only agent/status
	// handoffs or Huddle launch markers establish that a turn actually occurred.
	for _, c := range comments {
		if strings.Contains(c.Body, "<!-- huddle:auto-handoff") || strings.Contains(c.Body, "Status:") || strings.Contains(c.Body, "<!-- huddle:started -->") {
			snap.Started = true
		}
	}
	return snap, nil
}
