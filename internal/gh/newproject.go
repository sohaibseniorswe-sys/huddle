package gh

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// RepositoryCreator is used only by the explicitly approved new-project path.
// EnsurePrivateRepository must refuse an existing repo without the ownership
// marker, and must be safe to retry after creation succeeded but journaling failed.
type RepositoryCreator interface {
	EnsurePrivateRepository(context.Context, string, string, string) error
}

func (c CLI) EnsurePrivateRepository(ctx context.Context, repo, marker, branch string) error {
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`).MatchString(repo) || strings.HasSuffix(repo, "/.") || strings.HasSuffix(repo, "/..") {
		return fmt.Errorf("invalid repository")
	}
	if !ValidBranch(branch) || marker == "" {
		return fmt.Errorf("invalid new-project destination")
	}
	var metadata struct {
		Private       bool   `json:"private"`
		Description   string `json:"description"`
		DefaultBranch string `json:"default_branch"`
	}
	endpoint := "repos/" + repo
	err := c.apiJSON(ctx, "GET", endpoint, nil, &metadata)
	if err != nil {
		if !strings.Contains(err.Error(), "(HTTP 404)") {
			return err
		}
		// gh handles personal and organization owners and creates an initial README
		// commit. No checkout or project Git command is involved here.
		if _, err := c.run(ctx, "repo", "create", repo, "--private", "--add-readme", "--description", marker); err != nil {
			return err
		}
		if err := c.apiJSON(ctx, "GET", endpoint, nil, &metadata); err != nil {
			return err
		}
	}
	if !metadata.Private || metadata.Description != marker {
		return fmt.Errorf("refusing existing repository %s: private new-project ownership marker does not match", repo)
	}
	if metadata.DefaultBranch == "" {
		return fmt.Errorf("new repository has no initial commit; retry once initialization completes")
	}
	if branch == metadata.DefaultBranch {
		return nil
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := c.apiJSON(ctx, "GET", endpoint+"/git/ref/heads/"+branch, nil, &ref); err == nil {
		return nil
	} else if !strings.Contains(err.Error(), "(HTTP 404)") {
		return err
	}
	if err := c.apiJSON(ctx, "GET", endpoint+"/git/ref/heads/"+metadata.DefaultBranch, nil, &ref); err != nil {
		return err
	}
	return c.apiJSON(ctx, "POST", endpoint+"/git/refs", map[string]any{"ref": "refs/heads/" + branch, "sha": ref.Object.SHA}, nil)
}
