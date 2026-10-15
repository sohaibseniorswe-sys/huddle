package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// This file is what the start menu and plan mode need from GitHub (v3.6):
// listing and creating milestones, creating issues and changing an issue's
// body.

// Milestone is the subset of a GitHub milestone that Huddle uses.
type Milestone struct {
	Number       int
	Title        string
	Description  string
	OpenIssues   int // open issues, control issues included
	ClosedIssues int
}

// NewIssue is an issue to create.
type NewIssue struct {
	Title     string
	Body      string
	Milestone string   // the milestone's title; "" for none
	Labels    []string // labels that already exist on the repo
}

// ListMilestones returns repo's open milestones, oldest first, with
// `gh api repos/{owner}/{repo}/milestones?state=open`.
func (c CLI) ListMilestones(ctx context.Context, repo string) ([]Milestone, error) {
	out, err := c.run(ctx, "api", "--paginate", "repos/"+repo+"/milestones?state=open&sort=due_on&direction=asc&per_page=100")
	if err != nil {
		return nil, err
	}
	return parseMilestones(out)
}

// parseMilestones reads the milestones API's JSON. With --paginate, gh
// prints one JSON array per page one after the other, so they are read
// with a json.Decoder in a loop rather than with a single Unmarshal.
func parseMilestones(data []byte) ([]Milestone, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var out []Milestone
	for dec.More() {
		var page []struct {
			Number       int    `json:"number"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			OpenIssues   int    `json:"open_issues"`
			ClosedIssues int    `json:"closed_issues"`
		}
		if err := dec.Decode(&page); err != nil {
			return nil, fmt.Errorf("parse the milestones: %w", err)
		}
		for _, p := range page {
			out = append(out, Milestone{Number: p.Number, Title: p.Title, Description: p.Description,
				OpenIssues: p.OpenIssues, ClosedIssues: p.ClosedIssues})
		}
	}
	return out, nil
}

// CreateMilestone creates an open milestone on repo and returns its
// number.
func (c CLI) CreateMilestone(ctx context.Context, repo, title, description string) (int, error) {
	out, err := c.run(ctx, "api", "-X", "POST", "repos/"+repo+"/milestones",
		"-f", "title="+title, "-f", "description="+description, "--jq", ".number")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, fmt.Errorf("read the new milestone's number from %q: %w", out, err)
	}
	return n, nil
}

// CreateIssue creates an issue on repo and returns its number. The body
// goes on stdin, so its length and content never hit argv.
func (c CLI) CreateIssue(ctx context.Context, repo string, is NewIssue) (int, error) {
	args := []string{"issue", "create", "--repo", repo, "--title", is.Title, "--body-file", "-"}
	if is.Milestone != "" {
		args = append(args, "--milestone", is.Milestone)
	}
	for _, l := range is.Labels {
		args = append(args, "--label", l)
	}
	out, err := c.runStdin(ctx, is.Body, args...)
	if err != nil {
		return 0, err
	}
	return issueNumberFromURL(string(out))
}

// issueNumberFromURL reads the number at the end of the issue URL that
// `gh issue create` prints: ".../issues/12" gives 12.
func issueNumberFromURL(out string) (int, error) {
	url := strings.TrimSpace(out)
	if i := strings.LastIndex(url, "\n"); i >= 0 {
		url = url[i+1:] // gh may print notes before the URL
	}
	_, num, ok := strings.Cut(url, "/issues/")
	if !ok {
		return 0, fmt.Errorf("no issue URL in gh output %q", out)
	}
	n, err := strconv.Atoi(num)
	if err != nil {
		return 0, fmt.Errorf("read the issue number from %q: %w", url, err)
	}
	return n, nil
}

// EditBody replaces the body of issue number.
func (c CLI) EditBody(ctx context.Context, repo string, number int, body string) error {
	_, err := c.runStdin(ctx, body, "issue", "edit", strconv.Itoa(number), "--repo", repo, "--body-file", "-")
	return err
}
