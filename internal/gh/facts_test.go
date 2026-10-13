package gh

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestParsePullRequestsSummarisesChecks(t *testing.T) {
	// Shaped like gh pr list --json number,title,statusCheckRollup output.
	data := []byte(`[
	  {"number":73,"title":"Add /export","statusCheckRollup":[
	    {"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"SUCCESS"},
	    {"__typename":"CheckRun","name":"lint","status":"IN_PROGRESS","conclusion":""}]},
	  {"number":74,"title":"Map","statusCheckRollup":[
	    {"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE"},
	    {"__typename":"StatusContext","context":"ci/old","state":"SUCCESS"}]},
	  {"number":75,"title":"Docs","statusCheckRollup":[]}
	]`)
	prs, err := parsePullRequests(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []Checks{
		{Total: 2, Done: 1, Pending: 1},
		{Total: 2, Done: 2, Failed: 1},
		{},
	}
	for i, pr := range prs {
		if pr.Checks != want[i] {
			t.Errorf("PR #%d checks = %+v, want %+v", pr.Number, pr.Checks, want[i])
		}
	}
}

func TestFetchFacts(t *testing.T) {
	f := &Fake{
		Milestone: []IssueState{
			{Number: 58, Title: "Control", Labels: []string{"control", "state:dev-working"}},
			{Number: 59, Closed: true},
			{Number: 60},
		},
		PullRequests: []PullRequest{{Number: 73}},
		Merged:       &PullRequest{Number: 71},
		MainRun:      &Run{Status: "completed", Conclusion: "success"},
	}
	facts, err := FetchFacts(context.Background(), f, "o/r", "M3")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Issues) != 2 || !slices.Equal(facts.Closed(), []int{59}) {
		t.Errorf("issues = %+v, want #59 closed and #60 open, without the control issue", facts.Issues)
	}
	if len(facts.OpenPRs) != 1 || facts.LastMerged.Number != 71 || facts.MainCI() != "green" {
		t.Errorf("facts = %+v", facts)
	}

	f.FactsErr = errors.New("HTTP 502")
	if _, err := FetchFacts(context.Background(), f, "o/r", "M3"); err == nil {
		t.Error("want the gh error")
	}
}

func TestMainCI(t *testing.T) {
	for _, tt := range []struct {
		run  *Run
		want string
	}{
		{nil, "none"},
		{&Run{Status: "in_progress"}, "running"},
		{&Run{Status: "completed", Conclusion: "success"}, "green"},
		{&Run{Status: "completed", Conclusion: "failure"}, "red"},
	} {
		if got := (Facts{MainRun: tt.run}).MainCI(); got != tt.want {
			t.Errorf("MainCI(%+v) = %q, want %q", tt.run, got, tt.want)
		}
	}
}
