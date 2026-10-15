package start

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// Facts is what the start menu shows, read from GitHub before it opens.
type Facts struct {
	Repo       string           // "owner/name"
	Dir        string           // the local clone, shown in the bar
	Milestones []MilestoneEntry // the open milestones, for the picker
	// Resume is the control issue an interrupted run left behind; nil if
	// there is none.
	Resume *gh.Issue
}

// MilestoneEntry is one open milestone, as the picker shows it.
type MilestoneEntry struct {
	Title  string
	Issues int // its issues, control issues not counted
	Closed int // how many of them are closed
	// Control is its open control issue's number; 0 if it has none, and
	// Huddle creates one when it is chosen.
	Control int
}

// resumeStates are the labels that mean a run was interrupted: an agent
// was working or about to, or the PM waits for the human.
var resumeStates = []string{
	gh.StateDevReady, gh.StateDevWorking, gh.StatePMReady, gh.StatePMWorking, gh.StateHumanRequired,
}

// Load reads the start menu's facts from GitHub. queued is the milestone
// queue: if several control issues could be resumed, the first queued one
// wins, and otherwise the oldest.
func Load(ctx context.Context, client gh.Client, repo, dir string, queued []int) (Facts, error) {
	f := Facts{Repo: repo, Dir: dir}
	milestones, err := client.ListMilestones(ctx, repo)
	if err != nil {
		return f, fmt.Errorf("list the open milestones of %s: %w", repo, err)
	}
	open, err := client.ListIssues(ctx, repo, "open")
	if err != nil {
		return f, fmt.Errorf("list the open issues of %s: %w", repo, err)
	}

	// The open control issues, by milestone, and the resumable ones.
	controls := map[string]int{}
	var resumable []gh.Issue
	for _, is := range open {
		if !gh.IsControl(is.Labels, is.Body) || is.Milestone == "" {
			continue
		}
		if old, ok := controls[is.Milestone]; !ok || is.Number < old {
			controls[is.Milestone] = is.Number
		}
		if ResumeState(is) != "" {
			resumable = append(resumable, is)
		}
	}
	f.Resume = pickResume(resumable, queued)

	for _, m := range milestones {
		e := MilestoneEntry{Title: m.Title, Control: controls[m.Title], Closed: m.ClosedIssues}
		e.Issues = m.OpenIssues + m.ClosedIssues
		if e.Control != 0 {
			e.Issues-- // the control issue is not work
		}
		f.Milestones = append(f.Milestones, e)
	}
	return f, nil
}

// ResumeState returns the label on control issue is that makes it
// resumable, or "" if it has none.
func ResumeState(is gh.Issue) string {
	for _, s := range resumeStates {
		if slices.ContainsFunc(is.Labels, func(l string) bool { return strings.EqualFold(l, s) }) {
			return s
		}
	}
	return ""
}

// pickResume chooses the control issue to resume: the first queued one,
// or else the oldest. nil if there is none.
func pickResume(resumable []gh.Issue, queued []int) *gh.Issue {
	if len(resumable) == 0 {
		return nil
	}
	for _, q := range queued {
		for i := range resumable {
			if resumable[i].Number == q {
				return &resumable[i]
			}
		}
	}
	oldest := &resumable[0]
	for i := range resumable {
		if resumable[i].Number < oldest.Number {
			oldest = &resumable[i]
		}
	}
	return oldest
}

// FindMilestone returns the open milestone titled title.
func (f Facts) FindMilestone(title string) (MilestoneEntry, bool) {
	for _, m := range f.Milestones {
		if m.Title == title {
			return m, true
		}
	}
	return MilestoneEntry{}, false
}
