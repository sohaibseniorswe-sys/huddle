// Package gh wraps the GitHub CLI behind an interface so it can be faked in tests.
package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Issue is the subset of a GitHub issue that Huddle uses.
type Issue struct {
	Number    int
	Title     string
	Milestone string // empty if the issue has no milestone
	Labels    []string
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
}

// CLI implements Client by shelling out to gh.
type CLI struct{}

var _ Client = CLI{}

func (CLI) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
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
