package gh

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// StatePrefix is the prefix shared by all state labels.
const StatePrefix = "state:"

// The state labels (MVP_PLAN.md section 5).
const (
	StateDevReady      = "state:dev-ready"
	StateDevWorking    = "state:dev-working"
	StatePMReady       = "state:pm-ready"
	StatePMWorking     = "state:pm-working"
	StateHumanRequired = "state:human-required"
	StateDone          = "state:done"
)

// StateClosed is not a label. Control.State returns it when the control
// issue is closed without state:done. Whether the milestone is then
// finished depends on its other issues (see Control.OpenWork).
const StateClosed = "closed"

// statePriority decides between several state labels on the control issue.
// Agents may set a new label without removing the old one, so any label an
// agent or the human sets beats a leftover *-working label set by Huddle.
var statePriority = []string{
	StateHumanRequired,
	StateDone,
	StatePMReady,
	StateDevReady,
	StatePMWorking,
	StateDevWorking,
}

// Control is the control issue: the agents' shared channel.
type Control struct {
	Client Client
	Repo   string
	Number int
}

// State reads the control issue's state label and normalises it: if several
// known state labels are present, the first in priority order wins and every
// other state:* label is removed. It returns "" if there is no known state
// label (unknown state:* labels are then left alone). A closed issue
// changes nothing: State returns StateDone if it has state:done, and
// StateClosed otherwise.
func (c Control) State(ctx context.Context) (string, error) {
	is, err := c.Client.IssueView(ctx, c.Repo, c.Number)
	if err != nil {
		return "", fmt.Errorf("read labels on #%d: %w", c.Number, err)
	}
	if is.Closed {
		if slices.ContainsFunc(is.Labels, func(l string) bool { return strings.EqualFold(l, StateDone) }) {
			return StateDone, nil
		}
		return StateClosed, nil
	}
	labels := is.Labels
	state := ""
	for _, s := range statePriority {
		if slices.ContainsFunc(labels, func(l string) bool { return strings.EqualFold(l, s) }) {
			state = s
			break
		}
	}
	if state == "" {
		return "", nil
	}
	if extra := otherStates(labels, state); len(extra) > 0 {
		if err := c.Client.EditLabels(ctx, c.Repo, c.Number, nil, extra); err != nil {
			return "", fmt.Errorf("remove extra state labels from #%d: %w", c.Number, err)
		}
	}
	return state, nil
}

// SetState sets state on the control issue and removes every other state:*
// label, in one edit.
func (c Control) SetState(ctx context.Context, state string) error {
	is, err := c.Client.IssueView(ctx, c.Repo, c.Number)
	if err != nil {
		return fmt.Errorf("read labels on #%d: %w", c.Number, err)
	}
	labels := is.Labels
	var add []string
	if !slices.ContainsFunc(labels, func(l string) bool { return strings.EqualFold(l, state) }) {
		add = []string{state}
	}
	if err := c.Client.EditLabels(ctx, c.Repo, c.Number, add, otherStates(labels, state)); err != nil {
		return fmt.Errorf("set %s on #%d: %w", state, c.Number, err)
	}
	return nil
}

// RemoveLabel removes label from the control issue if it is there.
func (c Control) RemoveLabel(ctx context.Context, label string) error {
	is, err := c.Client.IssueView(ctx, c.Repo, c.Number)
	if err != nil {
		return fmt.Errorf("read labels on #%d: %w", c.Number, err)
	}
	labels := is.Labels
	i := slices.IndexFunc(labels, func(l string) bool { return strings.EqualFold(l, label) })
	if i < 0 {
		return nil
	}
	if err := c.Client.EditLabels(ctx, c.Repo, c.Number, nil, []string{labels[i]}); err != nil {
		return fmt.Errorf("remove %s from #%d: %w", label, c.Number, err)
	}
	return nil
}

// LatestComment returns the newest comment on the control issue; ok is false
// if it has none.
func (c Control) LatestComment(ctx context.Context) (comment Comment, ok bool, err error) {
	comments, err := c.Client.Comments(ctx, c.Repo, c.Number)
	if err != nil {
		return Comment{}, false, fmt.Errorf("read comments on #%d: %w", c.Number, err)
	}
	if len(comments) == 0 {
		return Comment{}, false, nil
	}
	return comments[len(comments)-1], true, nil
}

// OpenWork returns the numbers of milestone's open issues, leaving out
// control issues: the work still to do.
func (c Control) OpenWork(ctx context.Context, milestone string) ([]int, error) {
	issues, err := c.Client.MilestoneIssues(ctx, c.Repo, milestone)
	if err != nil {
		return nil, fmt.Errorf("list the issues of milestone %q: %w", milestone, err)
	}
	var open []int
	for _, is := range issues {
		if !is.Closed && is.Number != c.Number && !IsControl(is.Labels, is.Body) {
			open = append(open, is.Number)
		}
	}
	return open, nil
}

// Reopen reopens the control issue.
func (c Control) Reopen(ctx context.Context) error {
	if err := c.Client.ReopenIssue(ctx, c.Repo, c.Number); err != nil {
		return fmt.Errorf("reopen #%d: %w", c.Number, err)
	}
	return nil
}

// Comment posts body on the control issue.
func (c Control) Comment(ctx context.Context, body string) error {
	if err := c.Client.PostComment(ctx, c.Repo, c.Number, body); err != nil {
		return fmt.Errorf("comment on #%d: %w", c.Number, err)
	}
	return nil
}

// otherStates returns the state:* labels in labels other than keep.
func otherStates(labels []string, keep string) []string {
	var out []string
	for _, l := range labels {
		if strings.HasPrefix(strings.ToLower(l), StatePrefix) && !strings.EqualFold(l, keep) {
			out = append(out, l)
		}
	}
	return out
}
