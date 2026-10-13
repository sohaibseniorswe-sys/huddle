package gh

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func newControl(labels ...string) (Control, *Fake) {
	f := &Fake{Issues: []Issue{{Number: 3, Title: "Sandbox Control", Milestone: "Sandbox M1", Labels: labels}}}
	return Control{Client: f, Repo: "o/r", Number: 3}, f
}

// sameLabels compares label sets; GitHub does not keep label order.
func sameLabels(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func TestControlState(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   string
		left   []string // labels on the issue afterwards
	}{
		{"no state label", []string{"control"}, "", []string{"control"}},
		{"unknown state label only", []string{"control", "state:paused"}, "", []string{"control", "state:paused"}},
		{"single state", []string{"control", StateDevReady}, StateDevReady, []string{"control", StateDevReady}},
		{"working label only (interrupted run)", []string{"control", StatePMWorking}, StatePMWorking, []string{"control", StatePMWorking}},
		{"pm-ready beats dev-ready", []string{"control", StateDevReady, StatePMReady}, StatePMReady, []string{"control", StatePMReady}},
		{"done beats ready", []string{StatePMReady, "control", StateDone, StateDevReady}, StateDone, []string{"control", StateDone}},
		{"human-required beats everything", []string{StateDone, StateHumanRequired, StatePMReady, StateDevReady, "control"}, StateHumanRequired, []string{"control", StateHumanRequired}},
		{"agent label next to leftover working label", []string{"control", StateDevWorking, StatePMReady}, StatePMReady, []string{"control", StatePMReady}},
		{"agent label next to own working label", []string{"control", StatePMWorking, StateDevReady}, StateDevReady, []string{"control", StateDevReady}},
		{"unknown state removed when a known one wins", []string{"control", "state:paused", StateDevReady}, StateDevReady, []string{"control", StateDevReady}},
		{"label case differs", []string{"control", "State:PM-Ready"}, StatePMReady, []string{"control", "State:PM-Ready"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, f := newControl(tt.labels...)
			got, err := c.State(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("State() = %q, want %q", got, tt.want)
			}
			if !sameLabels(f.Issues[0].Labels, tt.left) {
				t.Errorf("labels after State() = %v, want %v", f.Issues[0].Labels, tt.left)
			}
		})
	}
}

func TestControlSetState(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		state  string
		left   []string
		edits  int
	}{
		{"from no state", []string{"control"}, StateDevWorking, []string{"control", StateDevWorking}, 1},
		{"replaces ready with working", []string{"control", StateDevReady}, StateDevWorking, []string{"control", StateDevWorking}, 1},
		{"removes every other state label", []string{StatePMReady, "control", StateDevReady, StateDevWorking, "state:paused"}, StatePMReady, []string{"control", StatePMReady}, 1},
		{"already set", []string{"control", StatePMReady}, StatePMReady, []string{"control", StatePMReady}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, f := newControl(tt.labels...)
			if err := c.SetState(context.Background(), tt.state); err != nil {
				t.Fatal(err)
			}
			if !sameLabels(f.Issues[0].Labels, tt.left) {
				t.Errorf("labels = %v, want %v", f.Issues[0].Labels, tt.left)
			}
			if f.Edits != tt.edits {
				t.Errorf("edits = %d, want %d", f.Edits, tt.edits)
			}
		})
	}
}

func TestControlRemoveLabel(t *testing.T) {
	c, f := newControl("control", StateDevWorking, StatePMReady)
	if err := c.RemoveLabel(context.Background(), StateDevWorking); err != nil {
		t.Fatal(err)
	}
	if want := []string{"control", StatePMReady}; !sameLabels(f.Issues[0].Labels, want) {
		t.Errorf("labels = %v, want %v", f.Issues[0].Labels, want)
	}
	// Removing a label that is not there is a no-op.
	if err := c.RemoveLabel(context.Background(), StateDevWorking); err != nil {
		t.Fatal(err)
	}
	if f.Edits != 1 {
		t.Errorf("edits = %d, want 1", f.Edits)
	}
}

func TestControlComments(t *testing.T) {
	ctx := context.Background()
	c, f := newControl("control")
	if _, ok, err := c.LatestComment(ctx); err != nil || ok {
		t.Fatalf("LatestComment() with no comments = ok %t, err %v; want false, nil", ok, err)
	}
	f.IssueComments = map[int][]Comment{3: {
		{Author: "codex", Body: "Status: Progress", CreatedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)},
		{Author: "claude", Body: "Status: Decision needed", CreatedAt: time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)},
	}}
	got, ok, err := c.LatestComment(ctx)
	if err != nil || !ok || got.Body != "Status: Decision needed" {
		t.Errorf("LatestComment() = %+v, %t, %v", got, ok, err)
	}
	if err := c.Comment(ctx, "handoff <!-- huddle:auto-handoff role=dev -->"); err != nil {
		t.Fatal(err)
	}
	got, _, _ = c.LatestComment(ctx)
	if got.Body != "handoff <!-- huddle:auto-handoff role=dev -->" {
		t.Errorf("LatestComment() after Comment() = %q", got.Body)
	}
}

func TestControlErrors(t *testing.T) {
	ctx := context.Background()
	c, f := newControl("control", StateDevReady)
	f.IssueErr = errors.New("gh: HTTP 502")
	if _, err := c.State(ctx); err == nil {
		t.Error("State() error = nil")
	}
	if err := c.SetState(ctx, StatePMReady); err == nil {
		t.Error("SetState() error = nil")
	}
	if err := c.RemoveLabel(ctx, StateDevReady); err == nil {
		t.Error("RemoveLabel() error = nil")
	}
	if _, _, err := c.LatestComment(ctx); err == nil {
		t.Error("LatestComment() error = nil")
	}
	if err := c.Comment(ctx, "x"); err == nil {
		t.Error("Comment() error = nil")
	}
}
