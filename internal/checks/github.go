package checks

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// ControlLabel marks the control issue.
const ControlLabel = "control"

// StatePrefix is the prefix shared by all state labels.
const StatePrefix = gh.StatePrefix

// Labels are the labels Huddle needs on the target repo (MVP_PLAN.md section 5).
var Labels = []gh.Label{
	{Name: ControlLabel, Color: "5319e7", Description: "Huddle control issue"},
	{Name: "state:dev-ready", Color: "7057ff", Description: "Huddle: waiting for the Dev agent"},
	{Name: "state:dev-working", Color: "1d76db", Description: "Huddle: Dev agent running"},
	{Name: "state:pm-ready", Color: "0e8a16", Description: "Huddle: waiting for the PM agent"},
	{Name: "state:pm-working", Color: "006b75", Description: "Huddle: PM agent running"},
	{Name: "state:human-required", Color: "d93f0b", Description: "Huddle: waiting for the human"},
	{Name: "state:done", Color: "c2e0c6", Description: "Huddle: milestone finished"},
}

// CheckGitHub verifies gh is logged in and repo is accessible.
func CheckGitHub(ctx context.Context, c gh.Client, repo string) error {
	if err := c.AuthStatus(ctx); err != nil {
		return fmt.Errorf("gh is not logged in (run `gh auth login`): %w", err)
	}
	if err := c.RepoView(ctx, repo); err != nil {
		return fmt.Errorf("cannot access %s: %w", repo, err)
	}
	return nil
}

// EnsureLabels creates any of Labels missing on repo and returns the names
// it created. Existing labels are left untouched, so it is idempotent.
func EnsureLabels(ctx context.Context, c gh.Client, repo string) ([]string, error) {
	existing, err := c.ListLabels(ctx, repo)
	if err != nil {
		return nil, fmt.Errorf("list labels on %s: %w", repo, err)
	}
	var created []string
	for _, l := range Labels {
		// GitHub label names are case-insensitive.
		if slices.ContainsFunc(existing, func(e string) bool { return strings.EqualFold(e, l.Name) }) {
			continue
		}
		if err := c.CreateLabel(ctx, repo, l); err != nil {
			return created, fmt.Errorf("create label %q on %s: %w", l.Name, repo, err)
		}
		created = append(created, l.Name)
	}
	return created, nil
}

// FindControlIssue returns the single open issue labelled "control".
// It fails if there are none, several, or the issue has no milestone.
func FindControlIssue(ctx context.Context, c gh.Client, repo string) (gh.Issue, error) {
	issues, err := c.ListOpenIssues(ctx, repo, ControlLabel)
	if err != nil {
		return gh.Issue{}, fmt.Errorf("list control issues on %s: %w", repo, err)
	}
	switch len(issues) {
	case 0:
		return gh.Issue{}, fmt.Errorf("no open issue labelled %q on %s", ControlLabel, repo)
	case 1:
	default:
		nums := make([]string, len(issues))
		for i, is := range issues {
			nums[i] = fmt.Sprintf("#%d", is.Number)
		}
		return gh.Issue{}, fmt.Errorf("want exactly one open issue labelled %q on %s, found %d: %s",
			ControlLabel, repo, len(issues), strings.Join(nums, ", "))
	}
	is := issues[0]
	if is.Milestone == "" {
		return gh.Issue{}, fmt.Errorf("control issue #%d has no milestone", is.Number)
	}
	return is, nil
}

// States returns the state:* labels on an issue, without the prefix.
func States(is gh.Issue) []string {
	var states []string
	for _, l := range is.Labels {
		if s, ok := strings.CutPrefix(l, StatePrefix); ok {
			states = append(states, s)
		}
	}
	return states
}
