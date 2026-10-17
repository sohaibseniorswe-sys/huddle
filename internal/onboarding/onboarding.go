// Package onboarding adopts existing project identities and begins next-milestone
// planning without changing a live build or writing to GitHub.
package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/gitx"
	"github.com/sohaibseniorswe-sys/huddle/internal/lifecycle"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// Adoption is a durable planning input snapshot. Respec marks open ordinary
// issues as candidates for explicit replacement links in the returned draft.
type Adoption struct {
	Respec bool       `json:"respec"`
	Issues []gh.Issue `json:"issues"`
}

// Adopt identifies by GitHub remote, falling back to an absolute directory when
// no remote is configured. Re-adopting the same identity never creates a second
// registry entry. All GitHub operations here are reads.
func Adopt(ctx context.Context, s store.Store, c gh.Client, dir string, respec bool) (store.Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return store.Project{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return store.Project{}, err
	}
	if !info.IsDir() {
		return store.Project{}, fmt.Errorf("adoption directory is not a directory")
	}
	repo, branch := "", ""
	if root, err := gitx.TopLevel(ctx, abs); err == nil {
		abs = root
		if remote, err := gitx.RemoteURL(ctx, abs, "origin"); err == nil {
			repo, err = checks.ParseRemoteURL(remote)
			if err != nil {
				return store.Project{}, err
			}
		}
		branch, _ = gitx.Branch(ctx, abs) // a detached checkout can still plan; publish requires a branch
	}
	ps, err := s.List()
	if err != nil {
		return store.Project{}, err
	}
	var p store.Project
	for _, candidate := range ps {
		if repo != "" && strings.EqualFold(repo, candidate.Repo) || repo == "" && candidate.Dir == abs {
			p = candidate
			break
		}
	}
	if p.ID == "" {
		id, err := store.NewID()
		if err != nil {
			return p, err
		}
		p = store.Project{ID: id, Name: filepath.Base(abs), Dir: abs, Repo: repo, Branch: branch, Phase: "planning", Status: "PLANNED"}
	}
	p.Dir = abs
	p.Archived = false
	if p.Branch == "" {
		p.Branch = branch
	}
	var source Adoption
	source.Respec = respec
	if repo != "" {
		if err := c.RepoView(ctx, repo); err != nil {
			return p, err
		}
		issues, err := c.ListIssues(ctx, repo, "open")
		if err != nil {
			return p, err
		}
		for _, is := range issues {
			if !gh.IsControl(is.Labels, is.Body) {
				source.Issues = append(source.Issues, is)
			}
		}
	}
	if err := s.Put(ctx, p); err != nil {
		return p, err
	}
	p, err = lifecycle.Refresh(ctx, c, s, p)
	if err != nil {
		return p, err
	}
	data, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return p, err
	}
	if err := s.Write(filepath.Join("drafts", p.ID, "adoption.json"), data); err != nil {
		return p, err
	}
	return p, s.Checkpoint(ctx, "Adopt existing project "+p.ID)
}

// BeginNext archives an already-published draft before starting N+1. Existing
// unpublished drafts are resumed instead, so repeated plan-next never loses work.
// The registry's live phase/control are preserved while planning proceeds.
func BeginNext(ctx context.Context, w planning.Workspace) (bool, error) {
	preview, err := publish.Review(w)
	if err != nil {
		_, exists, loadErr := w.Load()
		if loadErr != nil {
			return false, loadErr
		}
		if !exists {
			return false, nil
		}
		return false, nil // incomplete/unpublished content must remain resumable
	}
	data, err := os.ReadFile(filepath.Join(w.Dir(), "publish-"+preview.Revision+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var state struct {
		Complete bool `json:"complete"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return false, err
	}
	if !state.Complete {
		return false, nil
	}
	archive := filepath.Join(w.Store.Data(), "drafts", "archive-"+w.ProjectID+"-"+time.Now().UTC().Format("20060102-150405.000000000"))
	if err := os.Rename(w.Dir(), archive); err != nil {
		return false, err
	}
	return true, w.Store.Checkpoint(ctx, "Begin next milestone for "+w.ProjectID)
}
