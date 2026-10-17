// Package publish is the human-approved write path from draft specs to GitHub.
// A durable journal allows retries to retain completed work after partial failure.
package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/filelock"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// Preview is the exact draft and destination reviewed before approval.
type Preview struct {
	Draft    planning.Draft
	Project  store.Project
	Revision string
}

// Approval binds explicit human confirmation to an exact package/destination.
type Approval struct {
	Confirmed bool
	Revision  string
}

// Result records the created milestone, implementation issues and control.
type Result struct {
	Repo      string         `json:"repo"`
	Branch    string         `json:"branch"`
	Milestone int            `json:"milestone"`
	Title     string         `json:"title"`
	Issues    map[string]int `json:"issues"`
	Control   int            `json:"control"`
}

type journal struct {
	Revision string `json:"revision"`
	Specs    bool   `json:"specs"`
	Complete bool   `json:"complete"`
	Result   Result `json:"result"`
}

// Review validates content and loads its destination, performing no remote writes.
func Review(w planning.Workspace) (Preview, error) {
	d, exists, err := w.Load()
	if err != nil {
		return Preview{}, err
	}
	if !exists {
		return Preview{}, fmt.Errorf("no draft to publish")
	}
	if ps := d.Check(); len(ps) > 0 {
		var parts []string
		for _, p := range ps {
			parts = append(parts, p.String())
		}
		return Preview{}, fmt.Errorf("draft invalid: %s", strings.Join(parts, "; "))
	}
	p, err := w.Store.Read(w.ProjectID)
	if err != nil {
		return Preview{}, err
	}
	if d.Project != nil {
		if err := validateNewProject(*d.Project, p); err != nil {
			return Preview{}, err
		}
		p.Name, p.Dir, p.Repo, p.Branch = d.Project.Name, d.Project.Dir, d.Project.Repo, d.Project.Branch
	}
	if err := checks.ValidateRepo(p.Repo); err != nil {
		return Preview{}, fmt.Errorf("choose a repository before publishing: %w", err)
	}
	if !gh.ValidBranch(p.Branch) {
		return Preview{}, fmt.Errorf("choose a working branch before publishing")
	}
	b, err := json.Marshal(struct {
		Draft            planning.Draft
		ID, Repo, Branch string
	}{d, p.ID, p.Repo, p.Branch})
	if err != nil {
		return Preview{}, err
	}
	sum := sha256.Sum256(b)
	return Preview{Draft: d, Project: p, Revision: hex.EncodeToString(sum[:])}, nil
}

// Gate refuses another active control, including one found by marker only.
// Own is a journaled control from an interrupted retry, not an arbitrary bypass.
func Gate(ctx context.Context, c gh.Client, repo string, own int) error {
	open, err := c.ListIssues(ctx, repo, "open")
	if err != nil {
		return err
	}
	for _, is := range open {
		if is.Number == own || !gh.IsControl(is.Labels, is.Body) {
			continue
		}
		if slices.Contains(is.Labels, gh.StateDone) {
			continue
		}
		return fmt.Errorf("publish blocked: active control #%d on %s must be done first", is.Number, repo)
	}
	closed, err := c.ListIssues(ctx, repo, "closed")
	if err != nil {
		return err
	}
	// Only the newest closed control can represent interrupted current work.
	var latest gh.Issue
	for _, is := range closed {
		if gh.IsControl(is.Labels, is.Body) && is.Number > latest.Number {
			latest = is
		}
	}
	if latest.Number != 0 && latest.Number != own && latest.Milestone != "" && !slices.Contains(latest.Labels, gh.StateDone) {
		work, err := (gh.Control{Client: c, Repo: repo, Number: latest.Number}).OpenWork(ctx, latest.Milestone)
		if err != nil {
			return err
		}
		if len(work) > 0 {
			return fmt.Errorf("publish blocked: closed control #%d still has open milestone work", latest.Number)
		}
	}
	return nil
}

// Publish requires approval before any remote mutation, rechecks the gate, then
// writes specs, labels, milestone, ordered issues, adoption links and control.
func Publish(ctx context.Context, c gh.Client, w planning.Workspace, a Approval) (Result, error) {
	preview, err := Review(w)
	if err != nil {
		return Result{}, err
	}
	if !a.Confirmed || a.Revision == "" || a.Revision != preview.Revision {
		return Result{}, fmt.Errorf("explicit approval of the current draft and destination is required")
	}
	projectLock, err := filelock.Wait(ctx, filepath.Join(w.Store.Root, "publish-project-"+w.ProjectID+".lock"))
	if err != nil {
		return Result{}, err
	}
	defer projectLock.Close()
	lock, err := filelock.Wait(ctx, filepath.Join(w.Store.Root, "publish-"+store.RepoID(preview.Project.Repo)+".lock"))
	if err != nil {
		return Result{}, err
	}
	defer lock.Close()
	// Review again after acquiring the lock; approval must not drift while waiting.
	current, err := Review(w)
	if err != nil {
		return Result{}, err
	}
	if current.Revision != a.Revision {
		return Result{}, fmt.Errorf("draft changed after approval; review it again")
	}
	p, d := current.Project, current.Draft
	rel := filepath.Join("drafts", w.ProjectID, "publish-"+a.Revision+".json")
	j := journal{Revision: a.Revision, Result: Result{Repo: p.Repo, Branch: p.Branch, Title: d.Title, Issues: map[string]int{}}}
	data, err := os.ReadFile(filepath.Join(w.Store.Data(), rel))
	if err == nil {
		if err := json.Unmarshal(data, &j); err != nil {
			return j.Result, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return j.Result, err
	}
	if j.Revision != a.Revision || j.Result.Repo != p.Repo || j.Result.Branch != p.Branch {
		return Result{}, fmt.Errorf("publish journal does not match approved destination")
	}
	if j.Result.Issues == nil {
		j.Result.Issues = map[string]int{}
	}
	if j.Complete {
		return j.Result, nil
	}
	if d.Project != nil {
		if err := provision(ctx, c, w, p); err != nil {
			return j.Result, err
		}
	}
	if err := Gate(ctx, c, p.Repo, j.Result.Control); err != nil {
		return j.Result, err
	}
	checkpoint := func() error {
		b, err := json.MarshalIndent(j, "", "  ")
		if err != nil {
			return err
		}
		if err := w.Store.Write(rel, b); err != nil {
			return err
		}
		return w.Checkpoint(ctx)
	}
	if _, err := checks.EnsureLabels(ctx, c, p.Repo); err != nil {
		return j.Result, err
	}
	if !j.Specs {
		files := []gh.File{{Path: "docs/" + d.Slug + "/README.md", Content: d.README}}
		for _, is := range d.Issues {
			files = append(files, gh.File{Path: "docs/" + d.Slug + "/issues/" + is.ID + ".md", Content: is.Text})
		}
		if err := c.WriteFiles(ctx, p.Repo, p.Branch, files); err != nil {
			return j.Result, err
		}
		j.Specs = true
		if err := checkpoint(); err != nil {
			return j.Result, err
		}
	}
	if j.Result.Milestone == 0 {
		n, err := c.CreateMilestone(ctx, p.Repo, d.Title, "Spec: docs/"+d.Slug+"/README.md")
		if err != nil {
			return j.Result, err
		}
		j.Result.Milestone = n
		if err := checkpoint(); err != nil {
			return j.Result, err
		}
	}
	ordered, err := d.Order()
	if err != nil {
		return j.Result, err
	}
	for _, is := range ordered {
		if j.Result.Issues[is.ID] != 0 {
			continue
		}
		var deps []string
		for _, id := range is.DependsOn {
			deps = append(deps, fmt.Sprintf("#%d", j.Result.Issues[id]))
		}
		depends := "none"
		if len(deps) > 0 {
			depends = strings.Join(deps, ", ")
		}
		body := fmt.Sprintf("Spec: `docs/%s/issues/%s.md`. Read `docs/%s/README.md` first.\n\nDepends on: %s", d.Slug, is.ID, d.Slug, depends)
		n, err := c.CreateIssue(ctx, p.Repo, gh.NewIssue{Title: is.Title, Body: body, Milestone: d.Title})
		if err != nil {
			return j.Result, err
		}
		j.Result.Issues[is.ID] = n
		if err := checkpoint(); err != nil {
			return j.Result, err
		}
	}
	for _, is := range ordered {
		for _, old := range is.Supersedes {
			replacement := j.Result.Issues[is.ID]
			if old == replacement {
				return j.Result, fmt.Errorf("issue cannot supersede itself")
			}
			existing, err := c.IssueView(ctx, p.Repo, old)
			if err != nil {
				return j.Result, err
			}
			if existing.Closed {
				continue
			}
			body := fmt.Sprintf("Superseded by https://github.com/%s/issues/%d (normalized milestone spec).", p.Repo, replacement)
			if err := c.CloseIssue(ctx, p.Repo, old, body); err != nil {
				return j.Result, err
			}
		}
	}
	if j.Result.Control == 0 {
		if err := Gate(ctx, c, p.Repo, 0); err != nil {
			return j.Result, err
		}
		body := gh.ControlMarker + "\nControl channel for " + d.Title + ".\nWorking branch: `" + p.Branch + "`.\n\nFirst Dev task: implement `docs/" + d.Slug + "/README.md` and its issue specs."
		n, err := c.CreateIssue(ctx, p.Repo, gh.NewIssue{Title: d.Title + " · Control", Body: body, Milestone: d.Title, Labels: []string{"control", gh.StateDevReady}})
		if err != nil {
			return j.Result, err
		}
		j.Result.Control = n
		if err := checkpoint(); err != nil {
			return j.Result, err
		}
	}
	// Reload the latest record to retain any concurrently updated local attributes.
	p, err = w.Store.Read(p.ID)
	if err != nil {
		return j.Result, err
	}
	p.Phase = "published"
	p.Status = "PLANNED"
	p.Control = j.Result.Control
	p.Started = false
	if err := w.Store.Put(ctx, p); err != nil {
		return j.Result, err
	}
	j.Complete = true
	return j.Result, checkpoint()
}

// GateReview permits the exact journaled control on a publish retry, while
// retaining the ordinary active-control refusal for every other package.
func GateReview(ctx context.Context, c gh.Client, w planning.Workspace, p Preview) error {
	// A new repository does not exist yet. Provisioning verifies ownership and
	// the ordinary gate runs after creation, before milestone/spec writes.
	if p.Draft.Project != nil {
		original, err := w.Store.Read(w.ProjectID)
		if err != nil {
			return err
		}
		if original.Repo == "" {
			return nil
		}
	}
	own := 0
	path := filepath.Join(w.Dir(), "publish-"+p.Revision+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		var j journal
		if err := json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.Revision != p.Revision || j.Result.Repo != p.Project.Repo || j.Result.Branch != p.Project.Branch {
			return fmt.Errorf("publish journal destination mismatch")
		}
		own = j.Result.Control
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return Gate(ctx, c, p.Project.Repo, own)
}
