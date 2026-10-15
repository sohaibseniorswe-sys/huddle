package start

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

const testRepo = "sohaibseniorswe-sys/huddle-sandbox"

// twoMilestones is a plan of two milestones with three issues.
func twoMilestones() Plan {
	return Plan{Milestones: []PlannedMilestone{
		{Title: "M4", Description: "Maps", Issues: []PlannedIssue{{Title: "Export", Body: "CSV"}, {Title: "Map", Body: "A map"}}},
		{Title: "M5", Issues: []PlannedIssue{{Title: "Admin", Body: "Admin page"}}},
	}}
}

func TestApplyCreatesThePlanInOrder(t *testing.T) {
	f := &gh.Fake{Issues: []gh.Issue{{Number: 9, Title: "old", Closed: true}}}
	var reports []string
	created, err := Apply(context.Background(), f, testRepo, twoMilestones(), func(s string) { reports = append(reports, s) })
	if err != nil {
		t.Fatal(err)
	}
	wantLog := []string{
		"milestone M4",
		"issue #10 M4: Export",
		"issue #11 M4: Map",
		"issue #12 M4: M4 Control",
		"milestone M5",
		"issue #13 M5: Admin",
		"issue #14 M5: M5 Control",
	}
	if !slices.Equal(f.Log, wantLog) {
		t.Errorf("GitHub calls:\n%q\nwant\n%q", f.Log, wantLog)
	}
	if len(reports) != len(wantLog) {
		t.Errorf("reported %d steps, want %d: %q", len(reports), len(wantLog), reports)
	}
	want := []Created{{Milestone: "M4", Control: 12, Issues: []int{10, 11}}, {Milestone: "M5", Control: 14, Issues: []int{13}}}
	if len(created) != 2 || created[0].Control != 12 || created[1].Control != 14 ||
		!slices.Equal(created[0].Issues, want[0].Issues) || !slices.Equal(created[1].Issues, want[1].Issues) {
		t.Errorf("created = %+v, want %+v", created, want)
	}
	// The control issues are labelled and have the default first task.
	for _, n := range []int{12, 14} {
		is := f.Issues[slices.IndexFunc(f.Issues, func(is gh.Issue) bool { return is.Number == n })]
		if !gh.IsControl(is.Labels, "") || !strings.Contains(is.Body, "## First Dev task\n\nPrompt: Implement this milestone.") ||
			!strings.Contains(is.Body, gh.ControlMarker) {
			t.Errorf("control issue #%d: labels %q, body %q", n, is.Labels, is.Body)
		}
	}
	if f.Milestones[0].Description != "Maps" {
		t.Errorf("milestone description = %q", f.Milestones[0].Description)
	}
}

func TestApplyStopsAtTheFirstError(t *testing.T) {
	f := &gh.Fake{CreateErrAt: 5} // the second milestone
	created, err := Apply(context.Background(), f, testRepo, twoMilestones(), func(string) {})
	if err == nil || !strings.Contains(err.Error(), `create milestone "M5"`) {
		t.Fatalf("err = %v", err)
	}
	if len(created) != 1 || created[0].Milestone != "M4" {
		t.Errorf("created = %+v, want only M4", created)
	}
	if len(f.Log) != 4 {
		t.Errorf("GitHub calls after the error: %q", f.Log)
	}
}

func TestPrepareCreatesAControlIssue(t *testing.T) {
	cases := []struct {
		name, prompt, want string
	}{
		{"default prompt", "", "Prompt: Implement this milestone."},
		{"first message", "Start with #3, then the rest.", "Prompt: Start with #3, then the rest."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &gh.Fake{Issues: []gh.Issue{{Number: 3, Milestone: "M4", Title: "Work"}}}
			n, err := Prepare(context.Background(), f, testRepo, "M4", 0, tc.prompt)
			if err != nil {
				t.Fatal(err)
			}
			is := f.Issues[len(f.Issues)-1]
			if n != 4 || is.Number != 4 || is.Title != "M4 Control" || is.Milestone != "M4" {
				t.Fatalf("created #%d %+v", n, is)
			}
			if !strings.Contains(is.Body, "## First Dev task\n\n"+tc.want+"\n") {
				t.Errorf("body = %q, want it to contain %q", is.Body, tc.want)
			}
			if !slices.Contains(is.Labels, gh.ControlLabel) || !slices.Contains(is.Labels, gh.StateDevReady) {
				t.Errorf("labels = %q", is.Labels)
			}
		})
	}
}

func TestPrepareExistingControlIssue(t *testing.T) {
	body := ControlBody("M4", "")
	f := &gh.Fake{Issues: []gh.Issue{{Number: 8, Milestone: "M4", Labels: []string{"control", gh.StateHumanRequired}, Body: body}}}
	if _, err := Prepare(context.Background(), f, testRepo, "M4", 8, "Fix the CI first."); err != nil {
		t.Fatal(err)
	}
	is := f.Issues[0]
	if !strings.Contains(is.Body, "Prompt: Fix the CI first.\n") || strings.Contains(is.Body, DefaultPrompt) {
		t.Errorf("body = %q", is.Body)
	}
	if !strings.Contains(is.Body, gh.ControlMarker) {
		t.Errorf("the marker was lost: %q", is.Body)
	}
	if !slices.Equal(is.Labels, []string{"control", gh.StateDevReady}) {
		t.Errorf("labels = %q", is.Labels)
	}

	// Without a message the body is left alone.
	f.Log = nil
	if _, err := Prepare(context.Background(), f, testRepo, "M4", 8, ""); err != nil {
		t.Fatal(err)
	}
	if len(f.Log) != 0 {
		t.Errorf("GitHub calls: %q", f.Log)
	}
}

func TestWithPrompt(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"no section", "Hand-written control issue.", "Hand-written control issue.\n\n## First Dev task\n\nPrompt: Go\n"},
		{"section then heading", "Intro\n\n## First Dev task\n\nPrompt: old\nmore old\n## Notes\nkeep",
			"Intro\n\n## First Dev task\n\nPrompt: Go\n\n## Notes\nkeep"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WithPrompt(tc.body, "Go"); got != tc.want {
				t.Errorf("got %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	f := &gh.Fake{
		Milestones: []gh.Milestone{{Title: "M4", OpenIssues: 3, ClosedIssues: 2}, {Title: "M5", OpenIssues: 1}},
		Issues: []gh.Issue{
			{Number: 20, Milestone: "M4", Labels: []string{"control", gh.StatePMReady}},
			{Number: 30, Milestone: "M6", Labels: []string{"control", gh.StateDevWorking}},
			{Number: 31, Milestone: "M5", Labels: []string{"bug"}},
		},
	}
	facts, err := Load(context.Background(), f, testRepo, "~/sandbox", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []MilestoneEntry{{Title: "M4", Issues: 4, Closed: 2, Control: 20}, {Title: "M5", Issues: 1}}
	if !slices.Equal(facts.Milestones, want) {
		t.Errorf("milestones = %+v, want %+v", facts.Milestones, want)
	}
	if facts.Resume == nil || facts.Resume.Number != 20 {
		t.Errorf("resume = %+v, want the oldest, #20", facts.Resume)
	}

	// A queued control issue wins.
	facts, _ = Load(context.Background(), f, testRepo, "~/sandbox", []int{30, 20})
	if facts.Resume == nil || facts.Resume.Number != 30 {
		t.Errorf("resume = %+v, want the queued #30", facts.Resume)
	}

	// Nothing to resume.
	f.Issues = f.Issues[2:]
	facts, _ = Load(context.Background(), f, testRepo, "~/sandbox", nil)
	if facts.Resume != nil {
		t.Errorf("resume = %+v, want none", facts.Resume)
	}
}
