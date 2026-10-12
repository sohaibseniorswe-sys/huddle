package gh

import (
	"reflect"
	"testing"
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
