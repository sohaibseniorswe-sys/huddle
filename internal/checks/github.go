package checks

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// ControlLabel marks the control issue.
const ControlLabel = gh.ControlLabel

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

// FindControlIssue returns the single open control issue: the open issue
// with the "control" label or the <!-- huddle:control --> marker in its
// body. It fails if there are none, several, or the issue has no milestone.
func FindControlIssue(ctx context.Context, c gh.Client, repo string) (gh.Issue, error) {
	open, err := c.ListIssues(ctx, repo, "open")
	if err != nil {
		return gh.Issue{}, fmt.Errorf("list issues on %s: %w", repo, err)
	}
	var issues []gh.Issue
	for _, is := range open {
		if gh.IsControl(is.Labels, is.Body) {
			issues = append(issues, is)
		}
	}
	switch len(issues) {
	case 0:
		return gh.Issue{}, fmt.Errorf("%w on %s", ErrNoControlIssue, repo)
	case 1:
	default:
		slices.SortFunc(issues, func(a, b gh.Issue) int { return a.Number - b.Number })
		nums := make([]string, len(issues))
		for i, is := range issues {
			nums[i] = fmt.Sprintf("#%d", is.Number)
		}
		return gh.Issue{}, fmt.Errorf("want exactly one open control issue on %s, found %d: %s",
			repo, len(issues), strings.Join(nums, ", "))
	}
	is := issues[0]
	if is.Milestone == "" {
		return gh.Issue{}, fmt.Errorf("control issue #%d has no milestone", is.Number)
	}
	return is, nil
}

// ErrNoControlIssue is returned by FindControlIssue when no open issue is
// a control issue.
var ErrNoControlIssue = fmt.Errorf("no open issue labelled %q or marked %s", ControlLabel, gh.ControlMarker)

// closedControlLookback is how many recently closed control issues
// FindUnfinishedControl looks at.
const closedControlLookback = 5

// FindUnfinishedControl looks for a control issue that was closed too
// early: closed without state:done while other issues of its milestone are
// still open. It looks at the most recently closed control issues and
// returns the newest such one with the numbers of the open issues; ok is
// false if there is none. A closed control issue with state:done, or whose
// milestone has nothing open, is finished and is skipped.
func FindUnfinishedControl(ctx context.Context, c gh.Client, repo string) (is gh.Issue, open []int, ok bool, err error) {
	closed, err := c.ListIssues(ctx, repo, "closed")
	if err != nil {
		return gh.Issue{}, nil, false, fmt.Errorf("list closed issues on %s: %w", repo, err)
	}
	looked := 0
	for _, is := range closed {
		if !gh.IsControl(is.Labels, is.Body) || is.Milestone == "" {
			continue
		}
		if looked++; looked > closedControlLookback {
			break
		}
		if slices.ContainsFunc(is.Labels, func(l string) bool { return strings.EqualFold(l, gh.StateDone) }) {
			continue
		}
		control := gh.Control{Client: c, Repo: repo, Number: is.Number}
		open, err := control.OpenWork(ctx, is.Milestone)
		if err != nil {
			return gh.Issue{}, nil, false, err
		}
		if len(open) > 0 {
			return is, open, true, nil
		}
	}
	return gh.Issue{}, nil, false, nil
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
