package gh

import (
	"reflect"
	"testing"
	"time"
)

func TestParseIssues(t *testing.T) {
	data := []byte(`[
	  {"number":58,"title":"Palantir V3 Control","milestone":{"number":3,"title":"V3"},
	   "labels":[{"name":"control"},{"name":"state:dev-ready"}]},
	  {"number":7,"title":"No milestone","milestone":null,"labels":[{"name":"control"}]}
	]`)
	got, err := parseIssues(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []Issue{
		{Number: 58, Title: "Palantir V3 Control", Milestone: "V3", Labels: []string{"control", "state:dev-ready"}},
		{Number: 7, Title: "No milestone", Labels: []string{"control"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseIssues() = %+v, want %+v", got, want)
	}
}

func TestParseIssuesInvalid(t *testing.T) {
	if _, err := parseIssues([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseComments(t *testing.T) {
	data := []byte(`{"comments":[
	  {"id":"IC_1","author":{"login":"sohaibseniorswe-sys"},"authorAssociation":"OWNER","body":"Status: Progress\nDone: #1",
	   "createdAt":"2026-10-12T09:00:00Z","url":"https://github.com/o/r/issues/3#issuecomment-1","viewerDidAuthor":true}
	]}`)
	got, err := parseComments(data)
	if err != nil {
		t.Fatal(err)
	}
	want := []Comment{{Author: "sohaibseniorswe-sys", Body: "Status: Progress\nDone: #1",
		CreatedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC), URL: "https://github.com/o/r/issues/3#issuecomment-1"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseComments() = %+v, want %+v", got, want)
	}
	if got, err := parseComments([]byte(`{"comments":[]}`)); err != nil || len(got) != 0 {
		t.Errorf("parseComments(empty) = %v, %v", got, err)
	}
}

func TestIssuePaginationDoesNotLoseOldControlsOrIncludePRs(t *testing.T) {
	data := []byte(`[{"number":202,"title":"new","state":"open"},{"number":201,"pull_request":{"url":"pr"},"state":"open"}]
[{"number":1,"title":"old control","body":"<!-- huddle:control -->","labels":[{"name":"state:dev-ready"}],"state":"open"}]`)
	issues, err := parseIssues(data)
	if err != nil || len(issues) != 2 || issues[1].Number != 1 || !IsControl(issues[1].Labels, issues[1].Body) {
		t.Fatalf("paginated issues %+v %v", issues, err)
	}
	if _, err := parseIssues([]byte(`[] invalid`)); err == nil {
		t.Fatal("ignored malformed later page")
	}
}
