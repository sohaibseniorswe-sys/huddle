package gh

import (
	"context"
	"fmt"
	"slices"
)

// Fake is an in-memory Client for tests. The zero value is a logged-in user
// with access to every repo, no labels and no issues.
type Fake struct {
	AuthErr       error
	RepoErr       error
	ListErr       error
	CreateErr     error
	IssueErr      error // returned by the issue label and comment operations
	Labels        []string
	Issues        []Issue
	IssueComments map[int][]Comment // comments per issue number, oldest first
	Created       []string          // names of labels created, in order
	Edits         int               // number of EditLabels calls that changed something

	// The facts the run screen reads (see FetchFacts).
	FactsErr      error // returned by the four facts operations
	Milestone     []IssueState
	PullRequests  []PullRequest // open pull requests
	Merged        *PullRequest  // the last merged pull request; nil if none
	MainRun       *Run          // the latest run on any branch; nil if none
	FactsRequests int           // number of MilestoneIssues calls, i.e. fetches
}

var _ Client = (*Fake)(nil)

func (f *Fake) AuthStatus(context.Context) error { return f.AuthErr }

func (f *Fake) RepoView(context.Context, string) error { return f.RepoErr }

func (f *Fake) ListLabels(context.Context, string) ([]string, error) {
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	return slices.Clone(f.Labels), nil
}

func (f *Fake) CreateLabel(_ context.Context, _ string, l Label) error {
	if f.CreateErr != nil {
		return f.CreateErr
	}
	if slices.Contains(f.Labels, l.Name) {
		return fmt.Errorf("label %q already exists", l.Name)
	}
	f.Labels = append(f.Labels, l.Name)
	f.Created = append(f.Created, l.Name)
	return nil
}

func (f *Fake) ListOpenIssues(_ context.Context, _, label string) ([]Issue, error) {
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	var out []Issue
	for _, is := range f.Issues {
		if slices.Contains(is.Labels, label) {
			out = append(out, is)
		}
	}
	return out, nil
}

func (f *Fake) issue(number int) (*Issue, error) {
	if f.IssueErr != nil {
		return nil, f.IssueErr
	}
	for i := range f.Issues {
		if f.Issues[i].Number == number {
			return &f.Issues[i], nil
		}
	}
	return nil, fmt.Errorf("issue #%d not found", number)
}

func (f *Fake) IssueLabels(_ context.Context, _ string, number int) ([]string, error) {
	is, err := f.issue(number)
	if err != nil {
		return nil, err
	}
	return slices.Clone(is.Labels), nil
}

func (f *Fake) EditLabels(_ context.Context, _ string, number int, add, remove []string) error {
	is, err := f.issue(number)
	if err != nil {
		return err
	}
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	is.Labels = slices.DeleteFunc(is.Labels, func(l string) bool { return slices.Contains(remove, l) })
	for _, l := range add {
		if !slices.Contains(is.Labels, l) {
			is.Labels = append(is.Labels, l)
		}
	}
	f.Edits++
	return nil
}

func (f *Fake) Comments(_ context.Context, _ string, number int) ([]Comment, error) {
	if _, err := f.issue(number); err != nil {
		return nil, err
	}
	return slices.Clone(f.IssueComments[number]), nil
}

func (f *Fake) PostComment(_ context.Context, _ string, number int, body string) error {
	if _, err := f.issue(number); err != nil {
		return err
	}
	if f.IssueComments == nil {
		f.IssueComments = map[int][]Comment{}
	}
	f.IssueComments[number] = append(f.IssueComments[number], Comment{Author: "huddle", Body: body})
	return nil
}

func (f *Fake) MilestoneIssues(context.Context, string, string) ([]IssueState, error) {
	f.FactsRequests++
	return slices.Clone(f.Milestone), f.FactsErr
}

func (f *Fake) OpenPullRequests(context.Context, string) ([]PullRequest, error) {
	return slices.Clone(f.PullRequests), f.FactsErr
}

func (f *Fake) LastMergedPullRequest(context.Context, string) (PullRequest, bool, error) {
	if f.FactsErr != nil || f.Merged == nil {
		return PullRequest{}, false, f.FactsErr
	}
	return *f.Merged, true, nil
}

func (f *Fake) LatestRun(context.Context, string, string) (Run, bool, error) {
	if f.FactsErr != nil || f.MainRun == nil {
		return Run{}, false, f.FactsErr
	}
	return *f.MainRun, true, nil
}
