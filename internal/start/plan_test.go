package start

import (
	"strings"
	"testing"
)

func TestParsePlanOneMilestone(t *testing.T) {
	p, err := ParsePlan(`
milestone: Milestone 4 · Maps and exports
description: Maps on every page.
issue: Add the /export endpoint
body: GET /export returns a CSV.
issue: Add a map to the home page
body: The home page shows a map.
`)
	if err != nil {
		t.Fatal(err)
	}
	want := Plan{Milestones: []PlannedMilestone{{
		Title:       "Milestone 4 · Maps and exports",
		Description: "Maps on every page.",
		Issues: []PlannedIssue{
			{Title: "Add the /export endpoint", Body: "GET /export returns a CSV."},
			{Title: "Add a map to the home page", Body: "The home page shows a map."},
		},
	}}}
	if !equalPlans(p, want) {
		t.Errorf("got %+v\nwant %+v", p, want)
	}
	if got := p.Summary(); got != "1 milestone, 2 issues" {
		t.Errorf("Summary = %q", got)
	}
}

func TestParsePlanSeveralMilestones(t *testing.T) {
	p, err := ParsePlan(`milestone: M4
issue: A
body: a
milestone: M5
description: Second
issue: B
body: b
issue: C
body: c`)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Milestones) != 2 || p.Milestones[0].Title != "M4" || p.Milestones[1].Title != "M5" {
		t.Fatalf("milestones = %+v", p.Milestones)
	}
	if p.Milestones[0].Description != "" || p.Milestones[1].Description != "Second" {
		t.Errorf("descriptions = %q, %q", p.Milestones[0].Description, p.Milestones[1].Description)
	}
	if got := p.Summary(); got != "2 milestones, 3 issues" {
		t.Errorf("Summary = %q", got)
	}
}

func TestParsePlanMultiLineBody(t *testing.T) {
	p, err := ParsePlan(`milestone: M4
issue: Add the /export endpoint
body:
GET /export returns a CSV of all items.

Acceptance criteria:
- the header row names the columns
- issue: and body: inside a line are just text
  indented lines stay as they are

issue: Next
body: one line`)
	if err != nil {
		t.Fatal(err)
	}
	want := "GET /export returns a CSV of all items.\n\nAcceptance criteria:\n- the header row names the columns\n" +
		"- issue: and body: inside a line are just text\n  indented lines stay as they are"
	if got := p.Milestones[0].Issues[0].Body; got != want {
		t.Errorf("body = %q\nwant %q", got, want)
	}
	if got := p.Milestones[0].Issues[1].Title; got != "Next" {
		t.Errorf("second issue = %q", got)
	}
}

func TestParsePlanMissingFields(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"empty", "", `no "milestone:" line`},
		{"no milestone title", "milestone:\nissue: A\nbody: a", "milestone 1 has no title"},
		{"milestone without issues", "milestone: M4\ndescription: d", `milestone "M4" has no issues`},
		{"issue without title", "milestone: M4\nissue:\nbody: a", `milestone "M4": issue 1 has no title`},
		{"issue without body", "milestone: M4\nissue: A\nissue: B\nbody: b", `issue "A" has no body`},
		{"empty body", "milestone: M4\nissue: A\nbody:\n\n", `issue "A" has no body`},
		{"issue before milestone", "issue: A\nbody: a", `"issue:" comes before the first "milestone:" line`},
		{"text before milestone", "Here is the plan\nmilestone: M4", `"Here is the plan" comes before`},
		{"body before issue", "milestone: M4\nbody: a", `"body:" comes before its "issue:" line`},
		{"two bodies", "milestone: M4\nissue: A\nbody: a\nbody: b", `issue "A" has two "body:" lines`},
		{"text after a title", "milestone: M4\nissue: A\nmore title\nbody: a", "follows a title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParsePlan(tc.text)
			if err == nil {
				t.Fatalf("no error; plan %+v", p)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say %q", err, tc.want)
			}
			if len(p.Milestones) != 0 {
				t.Errorf("a failed parse returned milestones: %+v", p.Milestones)
			}
		})
	}
}

func TestPlanText(t *testing.T) {
	p := Plan{Milestones: []PlannedMilestone{{Title: "M4", Description: "Maps", Issues: []PlannedIssue{{Title: "A", Body: "line 1\nline 2"}}}}}
	want := "M4\nMaps\n\n  1. A\n     line 1\n     line 2"
	if got := p.Text(); got != want {
		t.Errorf("Text = %q\nwant %q", got, want)
	}
}

// equalPlans compares two plans field by field.
func equalPlans(a, b Plan) bool {
	if len(a.Milestones) != len(b.Milestones) {
		return false
	}
	for i := range a.Milestones {
		x, y := a.Milestones[i], b.Milestones[i]
		if x.Title != y.Title || x.Description != y.Description || len(x.Issues) != len(y.Issues) {
			return false
		}
		for j := range x.Issues {
			if x.Issues[j] != y.Issues[j] {
				return false
			}
		}
	}
	return true
}
