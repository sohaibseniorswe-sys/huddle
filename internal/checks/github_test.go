package checks

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

const testRepo = "sohaibseniorswe-sys/huddle-sandbox"

func labelNames() []string {
	names := make([]string, len(Labels))
	for i, l := range Labels {
		names[i] = l.Name
	}
	return names
}

func TestCheckGitHub(t *testing.T) {
	ctx := context.Background()
	if err := CheckGitHub(ctx, &gh.Fake{}, testRepo); err != nil {
		t.Errorf("CheckGitHub() = %v, want nil", err)
	}
	if err := CheckGitHub(ctx, &gh.Fake{AuthErr: errors.New("not logged in")}, testRepo); err == nil ||
		!strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("CheckGitHub() with auth error = %v, want login hint", err)
	}
	if err := CheckGitHub(ctx, &gh.Fake{RepoErr: errors.New("not found")}, testRepo); err == nil ||
		!strings.Contains(err.Error(), testRepo) {
		t.Errorf("CheckGitHub() with repo error = %v, want error naming repo", err)
	}
}

func TestEnsureLabels(t *testing.T) {
	ctx := context.Background()

	t.Run("creates all missing labels", func(t *testing.T) {
		f := &gh.Fake{Labels: []string{"bug"}}
		created, err := EnsureLabels(ctx, f, testRepo)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(created, labelNames()) {
			t.Errorf("created %v, want %v", created, labelNames())
		}
	})

	t.Run("creates only missing labels", func(t *testing.T) {
		f := &gh.Fake{Labels: []string{"control", "State:Dev-Ready"}}
		created, err := EnsureLabels(ctx, f, testRepo)
		if err != nil {
			t.Fatal(err)
		}
		want := labelNames()[2:]
		if !reflect.DeepEqual(created, want) {
			t.Errorf("created %v, want %v", created, want)
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		f := &gh.Fake{}
		if _, err := EnsureLabels(ctx, f, testRepo); err != nil {
			t.Fatal(err)
		}
		created, err := EnsureLabels(ctx, f, testRepo)
		if err != nil {
			t.Fatalf("second EnsureLabels() = %v", err)
		}
		if len(created) != 0 {
			t.Errorf("second run created %v, want nothing", created)
		}
		if len(f.Created) != len(Labels) {
			t.Errorf("labels created in total = %d, want %d", len(f.Created), len(Labels))
		}
	})

	t.Run("create fails, e.g. no write access", func(t *testing.T) {
		f := &gh.Fake{CreateErr: errors.New("HTTP 403")}
		if _, err := EnsureLabels(ctx, f, testRepo); err == nil {
			t.Error("EnsureLabels() = nil, want error")
		}
	})

	t.Run("list fails", func(t *testing.T) {
		f := &gh.Fake{ListErr: errors.New("boom")}
		if _, err := EnsureLabels(ctx, f, testRepo); err == nil {
			t.Error("EnsureLabels() = nil, want error")
		}
	})
}

func TestFindControlIssue(t *testing.T) {
	ctx := context.Background()
	control := gh.Issue{Number: 58, Title: "Palantir V3 Control", Milestone: "V3",
		Labels: []string{"control", "state:dev-ready"}}
	work := gh.Issue{Number: 59, Title: "Work", Milestone: "V3"}

	tests := []struct {
		name    string
		issues  []gh.Issue
		want    int
		wantErr string
	}{
		{"zero control issues", []gh.Issue{work}, 0, "no open issue"},
		{"one control issue", []gh.Issue{work, control}, 58, ""},
		{
			"several control issues",
			[]gh.Issue{control, {Number: 60, Milestone: "V3", Labels: []string{"control"}}, {Number: 61, Labels: []string{"control"}}},
			0, "found 3: #58, #60, #61",
		},
		{"control issue without milestone", []gh.Issue{{Number: 58, Labels: []string{"control"}}}, 0, "#58 has no milestone"},
		{"found by marker when the label is missing", []gh.Issue{work,
			{Number: 70, Milestone: "V4", Body: "Control issue for V4.\n\n" + gh.ControlMarker}}, 70, ""},
		{"label and marker on two issues", []gh.Issue{control, {Number: 70, Milestone: "V4", Body: gh.ControlMarker}}, 0, "found 2: #58, #70"},
		{"a closed control issue is not the open one", []gh.Issue{work, {Number: 50, Milestone: "V2", Labels: []string{"control"}, Closed: true}}, 0, "no open issue"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindControlIssue(ctx, &gh.Fake{Issues: tt.issues}, testRepo)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("FindControlIssue() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Number != tt.want {
				t.Errorf("FindControlIssue() = #%d, want #%d", got.Number, tt.want)
			}
		})
	}

	t.Run("list fails", func(t *testing.T) {
		if _, err := FindControlIssue(ctx, &gh.Fake{ListErr: errors.New("boom")}, testRepo); err == nil {
			t.Error("FindControlIssue() = nil, want error")
		}
	})
}

func TestStates(t *testing.T) {
	is := gh.Issue{Labels: []string{"control", "state:dev-ready", "bug"}}
	if got := States(is); !reflect.DeepEqual(got, []string{"dev-ready"}) {
		t.Errorf("States() = %v", got)
	}
	if got := States(gh.Issue{Labels: []string{"control"}}); len(got) != 0 {
		t.Errorf("States() = %v, want none", got)
	}
}

func TestFindUnfinishedControl(t *testing.T) {
	ctx := context.Background()
	f := &gh.Fake{
		Issues: []gh.Issue{
			{Number: 40, Milestone: "V1", Labels: []string{"control"}, Closed: true},                // older, also unfinished
			{Number: 50, Milestone: "V2", Body: gh.ControlMarker, Closed: true},                     // closed early, found by marker
			{Number: 55, Milestone: "V2b", Labels: []string{"control", gh.StateDone}, Closed: true}, // finished by label
			{Number: 56, Milestone: "V3", Labels: []string{"bug"}, Closed: true},                    // not a control issue
		},
		Milestone: []gh.IssueState{{Number: 51}, {Number: 52, Closed: true}, {Number: 53}},
	}
	is, open, ok, err := FindUnfinishedControl(ctx, f, testRepo)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || is.Number != 50 || !slices.Equal(open, []int{51, 53}) {
		t.Errorf("FindUnfinishedControl = #%d %v %v, want #50 with #51, #53 open", is.Number, open, ok)
	}

	// Nothing open in the milestone: every closed control issue is finished.
	f.Milestone = []gh.IssueState{{Number: 51, Closed: true}}
	if _, _, ok, err := FindUnfinishedControl(ctx, f, testRepo); ok || err != nil {
		t.Errorf("FindUnfinishedControl with nothing open = %v, %v; want none", ok, err)
	}
}
