// Package store owns the portable huddle-data repository. Project repositories
// are never changed here. Registry records are independent files; index.json is
// a small, rebuildable list of ids rather than a second copy of their state.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Project is the portable identity and last-known lifecycle of a project.
type Project struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Dir      string    `json:"dir,omitempty"`
	Repo     string    `json:"repo,omitempty"`
	Phase    string    `json:"phase"`
	Status   string    `json:"status,omitempty"`
	Control  int       `json:"control,omitempty"`
	Started  bool      `json:"started,omitempty"`
	Archived bool      `json:"archived,omitempty"`
	Touched  time.Time `json:"last_touched"`
}

// Store is Huddle's own git working cache below Root.
type Store struct{ Root string }

// Data returns the local huddle-data repository path.
func (s Store) Data() string { return filepath.Join(s.Root, "huddle-data") }

// Open clones an explicitly configured remote when necessary, or initializes a
// local store. Network/authentication failures are surfaced, never discarded.
func Open(ctx context.Context, root, remote string) (Store, error) {
	s := Store{Root: root}
	if _, err := os.Stat(filepath.Join(s.Data(), ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(root, 0700); err != nil {
			return s, err
		}
		if remote != "" {
			cmd := exec.CommandContext(ctx, "git", "clone", "--", remote, s.Data())
			if out, err := cmd.CombinedOutput(); err != nil {
				return s, fmt.Errorf("clone huddle-data: %w: %s", err, out)
			}
		} else {
			if err := os.MkdirAll(s.Data(), 0700); err != nil {
				return s, err
			}
			if _, err := s.git(ctx, "init", "--quiet"); err != nil {
				return s, err
			}
		}
	} else if err != nil {
		return s, err
	}
	for _, dir := range []string{"projects", "config", "drafts"} {
		if err := os.MkdirAll(filepath.Join(s.Data(), dir), 0700); err != nil {
			return s, err
		}
	}
	if err := s.Migrate(ctx); err != nil {
		return s, err
	}
	return s, nil
}

func (s Store) git(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{"-C", s.Data(), "-c", "user.name=Huddle", "-c", "user.email=huddle@localhost"}
	out, err := exec.CommandContext(ctx, "git", append(base, args...)...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("huddle-data git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

// Checkpoint commits pending store changes; identical content creates no commit.
// Draft authors call this at conversation checkpoints rather than on every token.
func (s Store) Checkpoint(ctx context.Context, message string) error {
	if _, err := s.git(ctx, "add", "--", "projects", "config", "drafts"); err != nil {
		return err
	}
	// index.json is outside those directories.
	if _, err := os.Stat(filepath.Join(s.Data(), "index.json")); err == nil {
		if _, err := s.git(ctx, "add", "--", "index.json"); err != nil {
			return err
		}
	}
	out, err := s.git(ctx, "diff", "--cached", "--name-only")
	if err != nil || len(out) == 0 {
		return err
	}
	_, err = s.git(ctx, "commit", "--quiet", "-m", message)
	return err
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// NewID returns a random identity that does not depend on name, repo or path.
func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// RepoID is the deterministic initial identity used only by legacy migration.
func RepoID(repo string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(repo)))
	return hex.EncodeToString(sum[:16])
}

// Write atomically replaces a file under the store, rejecting path traversal.
func (s Store) Write(rel string, data []byte) error {
	path := filepath.Join(s.Data(), rel)
	inside, err := filepath.Rel(s.Data(), path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("invalid store path %q", rel)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Read loads a registry record by internal id.
func (s Store) Read(id string) (Project, error) {
	var p Project
	if !validID.MatchString(id) {
		return p, fmt.Errorf("invalid project id %q", id)
	}
	b, err := os.ReadFile(filepath.Join(s.Data(), "projects", id+".json"))
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.ID != id {
		return p, fmt.Errorf("project identity mismatch: %q != %q", p.ID, id)
	}
	return p, nil
}

// List reads records in id order. It does no network calls or next-step analysis.
func (s Store) List() ([]Project, error) {
	files, err := filepath.Glob(filepath.Join(s.Data(), "projects", "*.json"))
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(files))
	for _, file := range files {
		p, err := s.Read(strings.TrimSuffix(filepath.Base(file), ".json"))
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// Put persists a registry record and index, and immediately commits the change.
func (s Store) Put(ctx context.Context, p Project) error {
	if !validID.MatchString(p.ID) {
		return fmt.Errorf("invalid project id %q", p.ID)
	}
	switch p.Phase {
	case "planning", "published", "building", "done":
	default:
		return fmt.Errorf("invalid phase %q", p.Phase)
	}
	if p.Repo != "" && !regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`).MatchString(p.Repo) {
		return fmt.Errorf("invalid repo %q", p.Repo)
	}
	p.Touched = time.Now().UTC()
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err = s.Write(filepath.Join("projects", p.ID+".json"), append(b, '\n')); err != nil {
		return err
	}
	ps, err := s.List()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	sort.Strings(ids)
	b, err = json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	if err = s.Write("index.json", append(b, '\n')); err != nil {
		return err
	}
	return s.Checkpoint(ctx, "Update project "+p.ID)
}

// FindRepo resolves a repo attribute to its stable internal identity.
func (s Store) FindRepo(repo string) (Project, bool, error) {
	ps, err := s.List()
	if err != nil {
		return Project{}, false, err
	}
	for _, p := range ps {
		if strings.EqualFold(p.Repo, repo) {
			return p, true, nil
		}
	}
	return Project{}, false, nil
}

// ConfigPath returns the settings path by internal id. New legacy-style callers
// get a deterministic id until their first save registers the project.
func (s Store) ConfigPath(repo string) string {
	id := RepoID(repo)
	if p, ok, err := s.FindRepo(repo); err == nil && ok {
		id = p.ID
	}
	return filepath.Join(s.Data(), "config", id+".yaml")
}

// Migrate preserves exact legacy YAML bytes and an immutable backup. Existing
// destinations win, so retries never overwrite subsequent user edits.
func (s Store) Migrate(ctx context.Context) error {
	files, err := filepath.Glob(filepath.Join(s.Root, "projects", "*.yaml"))
	if err != nil {
		return err
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var old struct {
			Repo string `yaml:"repo"`
			Dir  string `yaml:"dir"`
		}
		if err = yaml.Unmarshal(b, &old); err != nil {
			return fmt.Errorf("migrate %s: %w", file, err)
		}
		if old.Repo == "" {
			old.Repo = strings.Replace(strings.TrimSuffix(filepath.Base(file), ".yaml"), "_", "/", 1)
		}
		dest := s.ConfigPath(old.Repo)
		if _, err := os.Stat(dest); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		backup := filepath.Join(s.Root, "migration-backup", filepath.Base(file))
		if err = os.MkdirAll(filepath.Dir(backup), 0700); err != nil {
			return err
		}
		f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, err = f.Write(b)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err = s.Write(filepath.Join("config", RepoID(old.Repo)+".yaml"), b); err != nil {
			return err
		}
		if err = s.Put(ctx, Project{ID: RepoID(old.Repo), Name: old.Repo, Repo: old.Repo, Dir: old.Dir, Phase: "planning"}); err != nil {
			return err
		}
	}
	return nil
}
