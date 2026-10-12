package gh

import (
	"context"
	"fmt"
	"slices"
)

// Fake is an in-memory Client for tests. The zero value is a logged-in user
// with access to every repo, no labels and no issues.
type Fake struct {
	AuthErr   error
	RepoErr   error
	ListErr   error
	CreateErr error
	Labels    []string
	Issues    []Issue
	Created   []string // names of labels created, in order
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
