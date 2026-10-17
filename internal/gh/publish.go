package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// ValidBranch restricts publish refs to ordinary named branches, never HEAD or
// revision expressions. GitHub is still authoritative for branch existence.
func ValidBranch(branch string) bool {
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`).MatchString(branch) && branch != "HEAD" && !strings.Contains(branch, "..") && !strings.Contains(branch, "//") && !strings.HasSuffix(branch, "/") && !strings.HasSuffix(branch, ".lock") && !strings.HasSuffix(branch, ".")
}

// WriteFiles commits the exact approved spec package on a named remote working
// branch via GitHub's Git Data API. No project checkout git commands are run.
// The ref update is fast-forward only, so concurrent branch edits are preserved.
func (c CLI) WriteFiles(ctx context.Context, repo, branch string, files []File) error {
	if !ValidBranch(branch) {
		return fmt.Errorf("invalid publish branch %q", branch)
	}
	if len(files) == 0 {
		return fmt.Errorf("no spec files to publish")
	}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, "docs/") || path.Clean(f.Path) != f.Path {
			return fmt.Errorf("unsafe spec path %q", f.Path)
		}
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.apiJSON(ctx, "GET", "repos/"+repo+"/git/ref/heads/"+branch, nil, &ref); err != nil {
		return err
	}
	var parent struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	base := "repos/" + repo + "/git/"
	if err := c.apiJSON(ctx, "GET", base+"commits/"+ref.Object.SHA, nil, &parent); err != nil {
		return err
	}
	type entry struct {
		Path    string `json:"path"`
		Mode    string `json:"mode"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	entries := make([]entry, 0, len(files))
	for _, f := range files {
		entries = append(entries, entry{f.Path, "100644", "blob", f.Content})
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := c.apiJSON(ctx, "POST", base+"trees", map[string]any{"base_tree": parent.Tree.SHA, "tree": entries}, &tree); err != nil {
		return err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := c.apiJSON(ctx, "POST", base+"commits", map[string]any{"message": "Publish milestone specs", "tree": tree.SHA, "parents": []string{ref.Object.SHA}}, &commit); err != nil {
		return err
	}
	return c.apiJSON(ctx, "PATCH", base+"refs/heads/"+branch, map[string]any{"sha": commit.SHA, "force": false}, nil)
}

func (c CLI) apiJSON(ctx context.Context, method, endpoint string, input, output any) error {
	args := []string{"api", "-X", method, endpoint}
	stdin := ""
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		stdin = string(b)
		args = append(args, "--input", "-")
	}
	data, err := c.runStdin(ctx, stdin, args...)
	if err != nil {
		return err
	}
	if output != nil {
		if err := json.Unmarshal(data, output); err != nil {
			return fmt.Errorf("decode GitHub %s: %w", endpoint, err)
		}
	}
	return nil
}

// CloseIssue links a superseded issue to its replacement before closing it.
func (c CLI) CloseIssue(ctx context.Context, repo string, n int, body string) error {
	_, err := c.run(ctx, "issue", "close", fmt.Sprint(n), "--repo", repo, "--comment", body)
	return err
}
