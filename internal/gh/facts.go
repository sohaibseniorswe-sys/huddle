package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// MainBranch is the branch whose CI the run screen shows. Huddle's target
// repos all use main.
const MainBranch = "main"

// ControlLabel marks the control issue.
const ControlLabel = "control"

// ControlMarker in an issue's body also marks the control issue, so it is
// found when its label has been removed. Huddle puts it in the body of
// every control issue it creates.
const ControlMarker = "<!-- huddle:control -->"

// IsControl reports whether an issue with these labels and this body is a
// control issue: it has the control label or the marker.
func IsControl(labels []string, body string) bool {
	return slices.ContainsFunc(labels, func(l string) bool { return strings.EqualFold(l, ControlLabel) }) ||
		strings.Contains(body, ControlMarker)
}

// IssueState is an issue of a milestone, open or closed.
type IssueState struct {
	Number int
	Title  string
	Closed bool
	Labels []string
	Body   string
}

// PullRequest is a pull request with a summary of its checks.
type PullRequest struct {
	Number   int
	Title    string
	MergedAt time.Time // zero unless merged
	Checks   Checks
}

// Checks summarises a pull request's CI checks.
type Checks struct {
	Total   int // all checks
	Done    int // finished checks, passed or failed
	Failed  int // finished checks that did not pass
	Pending int // checks still queued or running
}

// Run is a GitHub Actions workflow run.
type Run struct {
	Status     string // "queued", "in_progress", "completed", ...
	Conclusion string // for a completed run: "success", "failure", ...
}

// Facts are the GitHub facts the run screen shows in its info line and in
// /status. They are read with FetchFacts.
type Facts struct {
	Issues     []IssueState  // the milestone's issues, without the control issue
	OpenPRs    []PullRequest // open pull requests, newest first
	LastMerged *PullRequest  // the most recently merged pull request; nil if none
	MainRun    *Run          // the latest workflow run on MainBranch; nil if none
	FetchedAt  time.Time
}

// Closed returns the numbers of the closed issues, in milestone order.
func (f Facts) Closed() []int {
	var out []int
	for _, is := range f.Issues {
		if is.Closed {
			out = append(out, is.Number)
		}
	}
	return out
}

// MainCI describes CI on MainBranch in one word: "green", "red",
// "running" or "none".
func (f Facts) MainCI() string {
	switch {
	case f.MainRun == nil:
		return "none"
	case f.MainRun.Status != "completed":
		return "running"
	case f.MainRun.Conclusion == "success":
		return "green"
	default:
		return "red"
	}
}

// FetchFacts reads the facts for milestone on repo. It runs several gh
// commands one after another, so it takes a second or two; the run screen
// calls it in the background.
func FetchFacts(ctx context.Context, c Client, repo, milestone string) (Facts, error) {
	var f Facts
	issues, err := c.MilestoneIssues(ctx, repo, milestone)
	if err != nil {
		return f, err
	}
	for _, is := range issues {
		// The control issue belongs to the milestone too, but it is the
		// agents' channel, not work to do.
		if !IsControl(is.Labels, is.Body) {
			f.Issues = append(f.Issues, is)
		}
	}
	if f.OpenPRs, err = c.OpenPullRequests(ctx, repo); err != nil {
		return f, err
	}
	merged, ok, err := c.LastMergedPullRequest(ctx, repo)
	if err != nil {
		return f, err
	}
	if ok {
		f.LastMerged = &merged
	}
	run, ok, err := c.LatestRun(ctx, repo, MainBranch)
	if err != nil {
		return f, err
	}
	if ok {
		f.MainRun = &run
	}
	return f, nil
}

func (c CLI) MilestoneIssues(ctx context.Context, repo, milestone string) ([]IssueState, error) {
	out, err := c.run(ctx, "issue", "list", "--repo", repo, "--milestone", milestone, "--state", "all",
		"--json", "number,title,state,labels,body", "--limit", "1000")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Number int
		Title  string
		State  string
		Labels []struct{ Name string }
		Body   string
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse gh issue list output: %w", err)
	}
	issues := make([]IssueState, len(raw))
	for i, r := range raw {
		issues[i] = IssueState{Number: r.Number, Title: r.Title, Closed: strings.EqualFold(r.State, "closed"), Body: r.Body}
		for _, l := range r.Labels {
			issues[i].Labels = append(issues[i].Labels, l.Name)
		}
	}
	// gh lists newest first; the milestone reads better oldest first.
	slices.SortFunc(issues, func(a, b IssueState) int { return a.Number - b.Number })
	return issues, nil
}

func (c CLI) OpenPullRequests(ctx context.Context, repo string) ([]PullRequest, error) {
	out, err := c.run(ctx, "pr", "list", "--repo", repo, "--state", "open",
		"--json", "number,title,statusCheckRollup", "--limit", "100")
	if err != nil {
		return nil, err
	}
	return parsePullRequests(out)
}

func (c CLI) LastMergedPullRequest(ctx context.Context, repo string) (PullRequest, bool, error) {
	// gh pr list sorts by creation; a search sorted by update time puts the
	// most recent merge first.
	out, err := c.run(ctx, "pr", "list", "--repo", repo, "--state", "merged", "--search", "sort:updated-desc",
		"--json", "number,title,mergedAt", "--limit", "1")
	if err != nil {
		return PullRequest{}, false, err
	}
	prs, err := parsePullRequests(out)
	if err != nil || len(prs) == 0 {
		return PullRequest{}, false, err
	}
	return prs[0], true, nil
}

func (c CLI) LatestRun(ctx context.Context, repo, branch string) (Run, bool, error) {
	out, err := c.run(ctx, "run", "list", "--repo", repo, "--branch", branch,
		"--json", "status,conclusion", "--limit", "1")
	if err != nil {
		return Run{}, false, err
	}
	var runs []Run
	if err := json.Unmarshal(out, &runs); err != nil {
		return Run{}, false, fmt.Errorf("parse gh run list output: %w", err)
	}
	if len(runs) == 0 {
		return Run{}, false, nil
	}
	return runs[0], true, nil
}

// parsePullRequests parses gh pr list JSON, summarising each
// statusCheckRollup. That list mixes two kinds of entries: check runs
// (GitHub Actions), with a status and a conclusion, and commit statuses
// (older CI services), with a single state.
func parsePullRequests(data []byte) ([]PullRequest, error) {
	var raw []struct {
		Number            int
		Title             string
		MergedAt          time.Time
		StatusCheckRollup []struct {
			Status     string // check run: QUEUED, IN_PROGRESS, COMPLETED
			Conclusion string // check run: SUCCESS, FAILURE, SKIPPED, ...
			State      string // commit status: PENDING, SUCCESS, FAILURE, ERROR
		}
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	prs := make([]PullRequest, len(raw))
	for i, r := range raw {
		prs[i] = PullRequest{Number: r.Number, Title: r.Title, MergedAt: r.MergedAt}
		for _, c := range r.StatusCheckRollup {
			prs[i].Checks.Total++
			result := strings.ToUpper(c.Conclusion)
			if c.State != "" {
				result = strings.ToUpper(c.State)
			} else if !strings.EqualFold(c.Status, "COMPLETED") {
				result = "PENDING"
			}
			switch result {
			case "PENDING", "EXPECTED", "":
				prs[i].Checks.Pending++
			case "SUCCESS", "SKIPPED", "NEUTRAL":
				prs[i].Checks.Done++
			default:
				prs[i].Checks.Done++
				prs[i].Checks.Failed++
			}
		}
	}
	return prs, nil
}
