// Package gh wraps the GitHub CLI behind an interface so it can be faked in tests.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Issue is the subset of a GitHub issue that Huddle uses.
type Issue struct {
	Number    int
	Title     string
	Milestone string // empty if the issue has no milestone
	Labels    []string
	Closed    bool // the issue is closed (only set by IssueView)
}

// Comment is an issue comment.
type Comment struct {
	Author    string
	Body      string
	CreatedAt time.Time
	URL       string
}

// Label is a GitHub label.
type Label struct {
	Name        string
	Color       string // hex without '#'
	Description string
}

// Client is the set of GitHub operations Huddle needs.
type Client interface {
	// AuthStatus succeeds if gh is logged in.
	AuthStatus(ctx context.Context) error
	// RepoView succeeds if repo ("owner/name") exists and is accessible.
	RepoView(ctx context.Context, repo string) error
	// ListLabels returns the names of all labels on repo.
	ListLabels(ctx context.Context, repo string) ([]string, error)
	// CreateLabel creates a label on repo.
	CreateLabel(ctx context.Context, repo string, l Label) error
	// ListOpenIssues returns the open issues on repo that carry label.
	ListOpenIssues(ctx context.Context, repo, label string) ([]Issue, error)
	// IssueView returns issue number's labels and whether it is closed.
	// Only Number, Labels and Closed are filled.
	IssueView(ctx context.Context, repo string, number int) (Issue, error)
	// EditLabels adds and removes labels on issue number in one call.
	// Removing a label the issue does not have is not an error.
	EditLabels(ctx context.Context, repo string, number int, add, remove []string) error
	// Comments returns the comments on issue number, oldest first.
	Comments(ctx context.Context, repo string, number int) ([]Comment, error)
	// PostComment posts a comment on issue number.
	PostComment(ctx context.Context, repo string, number int, body string) error
	// MilestoneIssues returns all issues of milestone, open and closed,
	// oldest first.
	MilestoneIssues(ctx context.Context, repo, milestone string) ([]IssueState, error)
	// OpenPullRequests returns the open pull requests on repo.
	OpenPullRequests(ctx context.Context, repo string) ([]PullRequest, error)
	// LastMergedPullRequest returns the most recently merged pull request;
	// ok is false if there is none.
	LastMergedPullRequest(ctx context.Context, repo string) (pr PullRequest, ok bool, err error)
	// LatestRun returns the latest workflow run on branch; ok is false if
	// there is none.
	LatestRun(ctx context.Context, repo, branch string) (run Run, ok bool, err error)
}

// CLI implements Client by shelling out to gh.
type CLI struct{}

var _ Client = CLI{}

func (c CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	return c.runStdin(ctx, "", args...)
}

func (CLI) runStdin(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("gh %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}

func (c CLI) AuthStatus(ctx context.Context) error {
	_, err := c.run(ctx, "auth", "status")
	return err
}

func (c CLI) RepoView(ctx context.Context, repo string) error {
	_, err := c.run(ctx, "repo", "view", repo, "--json", "nameWithOwner")
	return err
}

func (c CLI) ListLabels(ctx context.Context, repo string) ([]string, error) {
	out, err := c.run(ctx, "label", "list", "--repo", repo, "--json", "name", "--limit", "1000")
	if err != nil {
		return nil, err
	}
	var labels []struct{ Name string }
	if err := json.Unmarshal(out, &labels); err != nil {
		return nil, fmt.Errorf("parse gh label list output: %w", err)
	}
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names, nil
}

func (c CLI) CreateLabel(ctx context.Context, repo string, l Label) error {
	_, err := c.run(ctx, "label", "create", l.Name, "--repo", repo,
		"--color", l.Color, "--description", l.Description)
	return err
}

func (c CLI) ListOpenIssues(ctx context.Context, repo, label string) ([]Issue, error) {
	out, err := c.run(ctx, "issue", "list", "--repo", repo, "--state", "open",
		"--label", label, "--json", "number,title,milestone,labels", "--limit", "100")
	if err != nil {
		return nil, err
	}
	return parseIssues(out)
}

func (c CLI) IssueView(ctx context.Context, repo string, number int) (Issue, error) {
	out, err := c.run(ctx, "issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "labels,state")
	if err != nil {
		return Issue{}, err
	}
	return parseIssueView(number, out)
}

// parseIssueView reads `gh issue view --json labels,state` output. gh
// prints the state as "OPEN" or "CLOSED".
func parseIssueView(number int, data []byte) (Issue, error) {
	var raw struct {
		State  string
		Labels []struct{ Name string }
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Issue{}, fmt.Errorf("parse gh issue view output: %w", err)
	}
	is := Issue{Number: number, Closed: strings.EqualFold(raw.State, "closed")}
	for _, l := range raw.Labels {
		is.Labels = append(is.Labels, l.Name)
	}
	return is, nil
}

func (c CLI) EditLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	if len(add) == 0 && len(remove) == 0 {
		return nil
	}
	args := []string{"issue", "edit", strconv.Itoa(number), "--repo", repo}
	for _, l := range add {
		args = append(args, "--add-label", l)
	}
	for _, l := range remove {
		args = append(args, "--remove-label", l)
	}
	_, err := c.run(ctx, args...)
	return err
}

func (c CLI) Comments(ctx context.Context, repo string, number int) ([]Comment, error) {
	out, err := c.run(ctx, "issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "comments")
	if err != nil {
		return nil, err
	}
	return parseComments(out)
}

func (c CLI) PostComment(ctx context.Context, repo string, number int, body string) error {
	// The body goes on stdin, so its length and content never hit argv.
	_, err := c.runStdin(ctx, body, "issue", "comment", strconv.Itoa(number), "--repo", repo, "--body-file", "-")
	return err
}

func parseComments(data []byte) ([]Comment, error) {
	var raw struct {
		Comments []struct {
			Author    struct{ Login string }
			Body      string
			CreatedAt time.Time
			URL       string
		}
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gh issue view output: %w", err)
	}
	comments := make([]Comment, len(raw.Comments))
	for i, r := range raw.Comments {
		comments[i] = Comment{Author: r.Author.Login, Body: r.Body, CreatedAt: r.CreatedAt, URL: r.URL}
	}
	return comments, nil
}

func parseIssues(data []byte) ([]Issue, error) {
	var raw []struct {
		Number    int
		Title     string
		Milestone *struct{ Title string }
		Labels    []struct{ Name string }
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse gh issue list output: %w", err)
	}
	issues := make([]Issue, len(raw))
	for i, r := range raw {
		issues[i] = Issue{Number: r.Number, Title: r.Title}
		if r.Milestone != nil {
			issues[i].Milestone = r.Milestone.Title
		}
		for _, l := range r.Labels {
			issues[i].Labels = append(issues[i].Labels, l.Name)
		}
	}
	return issues, nil
}
