package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/planning"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

func validateNewProject(n planning.NewProject, p store.Project) error {
	if strings.TrimSpace(n.Name) == "" {
		return fmt.Errorf("new project name is required")
	}
	if !filepath.IsAbs(n.Dir) || filepath.Clean(n.Dir) != n.Dir || n.Dir == filepath.VolumeName(n.Dir)+string(filepath.Separator) {
		return fmt.Errorf("new project directory must be an absolute, clean, non-root path")
	}
	if err := checks.ValidateRepo(n.Repo); err != nil {
		return err
	}
	if !gh.ValidBranch(n.Branch) {
		return fmt.Errorf("invalid new project branch")
	}
	if p.Repo != "" && (p.Repo != n.Repo || p.Dir != n.Dir || p.Branch != n.Branch || p.Name != n.Name) {
		return fmt.Errorf("new-project proposal cannot replace an existing destination")
	}
	return nil
}

type setupJournal struct {
	Destination planning.NewProject `json:"destination"`
	Ready       bool                `json:"ready"`
}

// provision is called only after exact approval under the repository publish
// lock. The setup intent is durable before creating any directory or repository.
func provision(ctx context.Context, c gh.Client, w planning.Workspace, p store.Project) error {
	creator, ok := c.(gh.RepositoryCreator)
	if !ok {
		return fmt.Errorf("GitHub client does not support new-project creation")
	}
	n := planning.NewProject{Name: p.Name, Dir: p.Dir, Repo: p.Repo, Branch: p.Branch}
	rel := filepath.Join("drafts", w.ProjectID, "setup.json")
	j := setupJournal{Destination: n}
	data, err := os.ReadFile(filepath.Join(w.Store.Data(), rel))
	if err == nil {
		if err := json.Unmarshal(data, &j); err != nil {
			return err
		}
		if j.Destination != n {
			return fmt.Errorf("new-project setup already started for another destination; resume the original proposal")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		if _, err := os.Lstat(p.Dir); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("new project directory already exists or cannot be checked: %s", p.Dir)
		}
		if _, err := os.Lstat(stagingPath(p)); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("bootstrap staging directory already exists or cannot be checked: %s", stagingPath(p))
		}
		if p.Repo != "" {
			original, err := w.Store.Read(w.ProjectID)
			if err != nil {
				return err
			}
			if original.Repo != "" {
				return fmt.Errorf("new-project creation requires an unbound project identity")
			}
		}
		b, err := json.Marshal(j)
		if err != nil {
			return err
		}
		if err := w.Store.Write(rel, b); err != nil {
			return err
		}
		if err := w.Checkpoint(ctx); err != nil {
			return err
		}
	}
	if j.Ready {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.Dir), 0700); err != nil {
		return err
	}
	if err := os.Mkdir(p.Dir, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(p.Dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("new project directory must not be a symlink")
	}
	// Name/directory are now real outputs; retain the internal id and planning phase.
	original, err := w.Store.Read(w.ProjectID)
	if err != nil {
		return err
	}
	original.Name, original.Dir = p.Name, p.Dir
	if err := w.Store.Put(ctx, original); err != nil {
		return err
	}
	marker := "Huddle new project " + w.ProjectID
	if err := creator.EnsurePrivateRepository(ctx, p.Repo, marker, p.Branch); err != nil {
		return err
	}
	if err := bootstrap(ctx, p); err != nil {
		return err
	}
	cfg, _, err := config.LoadID(w.Store.Root, w.ProjectID)
	if err != nil {
		return err
	}
	cfg.Repo, cfg.Dir = p.Repo, p.Dir
	original.Repo, original.Branch = p.Repo, p.Branch
	if err := w.Store.Put(ctx, original); err != nil {
		return err
	}
	if err := config.SaveID(w.Store.Root, w.ProjectID, cfg); err != nil {
		return err
	}
	j.Ready = true
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := w.Store.Write(rel, b); err != nil {
		return err
	}
	return w.Checkpoint(ctx)
}

// bootstrap is the sole setup-only project Git exception. A clone is staged
// separately and renamed over the empty approved directory. Interrupted clones
// never overwrite user files; a completed clone is recognized on retry.
func bootstrap(ctx context.Context, p store.Project) error {
	remote := "https://github.com/" + p.Repo + ".git"
	verify := func(dir string) error {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid bootstrap checkout at %s", dir)
		}
		info, err = os.Lstat(filepath.Join(dir, ".git"))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("bootstrap checkout is missing its own .git directory")
		}
		cmd := exec.CommandContext(ctx, "git", "-C", dir, "config", "--local", "--get", "remote.origin.url")
		out, err := cmd.Output()
		if err != nil || strings.TrimSpace(string(out)) != remote {
			return fmt.Errorf("checkout at %s does not match approved repository", dir)
		}
		cmd = exec.CommandContext(ctx, "git", "-C", dir, "symbolic-ref", "--short", "HEAD")
		out, err = cmd.Output()
		if err != nil || strings.TrimSpace(string(out)) != p.Branch {
			return fmt.Errorf("checkout does not match approved branch")
		}
		return nil
	}
	entries, err := os.ReadDir(p.Dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return verify(p.Dir)
	}
	staging := stagingPath(p)
	// Setup checked that this staging path did not exist before committing its
	// ownership intent. A killed clone can leave incomplete temporary files.
	if _, err := os.Lstat(staging); err == nil {
		if err := verify(staging); err != nil {
			if err := os.RemoveAll(staging); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(staging); errors.Is(err, os.ErrNotExist) {
		cmd := exec.CommandContext(ctx, "git", "clone", "--branch", p.Branch, "--", remote, staging)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("bootstrap clone: %w: %s", err, out)
		}
	} else if err != nil {
		return err
	}
	if err := verify(staging); err != nil {
		return err
	}
	// Remove only the still-empty directory; Remove refuses any user content.
	if err := os.Remove(p.Dir); err != nil {
		return err
	}
	return os.Rename(staging, p.Dir)
}

func stagingPath(p store.Project) string { return p.Dir + ".huddle-" + p.ID + "-clone" }
