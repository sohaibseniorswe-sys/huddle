// Package planning persists companion-authored milestone drafts and checks the
// embedded Milestone Spec Standard before any package can be published.
package planning

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
)

// Standard is the embedded contract shown to the read-only planning companion.
//
//go:embed standard.md
var Standard string

// Instructions replaces the legacy planning/publishing proposal protocol for v4.
func Instructions(milestone string) string {
	base, _, _ := strings.Cut(companion.Instructions(milestone), "\nPlans:")
	return base + "\n\nDraft one milestone at a time. Use <draft> blocks instead of <plan> blocks. Huddle saves drafts locally; saving never publishes.\n\n" + Standard
}

// NewProject is a proposed destination, created only by approved publishing.
// Paths are absolute so a resumed session cannot reinterpret the destination.
type NewProject struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
}

// Draft is one unpublished milestone's portable spec package.
type Draft struct {
	Project *NewProject `json:"project,omitempty"`
	Title   string      `json:"title"`
	Slug    string      `json:"slug"`
	README  string      `json:"readme"`
	Issues  []Issue     `json:"issues"`
}

// Issue carries product requirements, evidence and dependencies, not workflow.
type Issue struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Text       string   `json:"text"`
	DependsOn  []string `json:"depends_on"`
	Supersedes []int    `json:"supersedes,omitempty"`
}

// Problem identifies a failed standard check without discarding the draft.
type Problem struct {
	Path    string
	Message string
}

func (p Problem) String() string { return p.Path + ": " + p.Message }

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
var process = regexp.MustCompile(`(?i)\b(open|create|merge|submit)\s+(a|the|your)\s+(pull request|PR)\b|\b(push|commit)\s+(your|the)\s+(code|changes|branch)\b|\b(hand over|ask the PM|ask the Dev|wait for review)\b`)

// Parse rejects misspelled keys and trailing data. Semantic problems are Check's
// responsibility so incomplete drafts remain editable and resumable.
func Parse(text string) (Draft, error) {
	var d Draft
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return d, fmt.Errorf("draft must contain exactly one JSON object")
	}
	return d, nil
}

// Check returns every missing section, process instruction and dependency error.
func (d Draft) Check() []Problem {
	var problems []Problem
	add := func(path, msg string) { problems = append(problems, Problem{path, msg}) }
	if strings.TrimSpace(d.Title) == "" {
		add("title", "required")
	}
	if !slug.MatchString(d.Slug) {
		add("slug", "use lowercase letters, digits and hyphens")
	}
	for _, section := range []string{"Objective", "Definition of Done"} {
		if !hasSection(d.README, section) {
			add("README", "missing or empty "+section)
		}
	}
	if len(d.Issues) == 0 {
		add("issues", "at least one issue is required")
	}
	known := map[string]bool{}
	for _, is := range d.Issues {
		if !slug.MatchString(is.ID) {
			add(is.ID, "invalid issue id")
		}
		if known[is.ID] {
			add(is.ID, "duplicate issue id")
		}
		known[is.ID] = true
		if strings.TrimSpace(is.Title) == "" {
			add(is.ID, "title required")
		}
		for _, section := range []string{"Objective", "Requirements", "Success criteria", "Evidence required", "Dependencies"} {
			if !hasSection(is.Text, section) {
				add(is.ID, "missing or empty "+section)
			}
		}
		if match := process.FindString(is.Title + "\n" + is.Text); match != "" {
			add(is.ID, "process language: "+match)
		}
		seen := map[string]bool{}
		for _, dep := range is.DependsOn {
			if seen[dep] {
				add(is.ID, "duplicate dependency "+dep)
			}
			seen[dep] = true
		}
		for _, n := range is.Supersedes {
			if n <= 0 {
				add(is.ID, "superseded issue number must be positive")
			}
		}
	}
	for _, is := range d.Issues {
		for _, dep := range is.DependsOn {
			if !known[dep] {
				add(is.ID, "unknown dependency "+dep)
			}
		}
	}
	if _, err := d.Order(); err != nil {
		add("dependencies", err.Error())
	}
	return problems
}

// hasSection recognizes Markdown ATX headings and requires content before the
// next heading, avoiding false passes from a heading-only placeholder.
func hasSection(text, want string) bool {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		heading := strings.Trim(strings.TrimSpace(line), "# ")
		if !strings.EqualFold(heading, want) {
			continue
		}
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(strings.TrimSpace(next), "#") {
				break
			}
			if strings.TrimSpace(next) != "" {
				return true
			}
		}
	}
	return false
}

// Order returns issue specs in stable dependency order, refusing unknown ids or
// cycles rather than guessing an order that would publish invalid references.
func (d Draft) Order() ([]Issue, error) {
	byID := map[string]Issue{}
	for _, is := range d.Issues {
		if _, ok := byID[is.ID]; ok {
			return nil, fmt.Errorf("duplicate issue id %s", is.ID)
		}
		byID[is.ID] = is
	}
	state := map[string]int{}
	var ordered []Issue
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("dependency cycle at %s", id)
		}
		if state[id] == 2 {
			return nil
		}
		is, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown dependency %s", id)
		}
		state[id] = 1
		for _, dep := range is.DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, is)
		return nil
	}
	for _, is := range d.Issues {
		if err := visit(is.ID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// Workspace scopes planning storage to an existing stable project identity.
type Workspace struct {
	Store     store.Store
	ProjectID string
}

// Dir is this project's draft directory; methods validate identity before writes.
func (w Workspace) Dir() string { return filepath.Join(w.Store.Data(), "drafts", w.ProjectID) }

// History is portable planning memory, separate from local runtime logs.
func (w Workspace) History() companion.History {
	return companion.History{Path: filepath.Join(w.Dir(), "conversation.jsonl")}
}

// Load resumes a saved spec package. Missing drafts are empty, not errors.
func (w Workspace) Load() (Draft, bool, error) {
	if _, err := w.Store.Read(w.ProjectID); err != nil {
		return Draft{}, false, err
	}
	b, err := os.ReadFile(filepath.Join(w.Dir(), "draft.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Draft{}, false, nil
	}
	if err != nil {
		return Draft{}, false, err
	}
	d, err := Parse(string(b))
	return d, true, err
}

// Save writes a whole response/package; Checkpoint commits the completed exchange.
// Invalid drafts persist for editing,
// but unsafe filenames are rejected before filesystem writes.
func (w Workspace) Save(ctx context.Context, d Draft) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := w.Store.Read(w.ProjectID); err != nil {
		return err
	}
	for _, is := range d.Issues {
		if !slug.MatchString(is.ID) {
			return fmt.Errorf("invalid issue id %q", is.ID)
		}
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	base := filepath.Join("drafts", w.ProjectID)
	if err := w.Store.Write(filepath.Join(base, "draft.json"), append(b, '\n')); err != nil {
		return err
	}
	if err := w.Store.Write(filepath.Join(base, "README.md"), []byte(d.README)); err != nil {
		return err
	}
	for _, is := range d.Issues {
		if err := w.Store.Write(filepath.Join(base, "issues", is.ID+".md"), []byte(is.Text)); err != nil {
			return err
		}
	}
	// Remove specs dropped from the editable package; the JSON manifest and
	// rendered files must describe the same draft after each checkpoint.
	current := map[string]bool{}
	for _, is := range d.Issues {
		current[is.ID+".md"] = true
	}
	files, err := os.ReadDir(filepath.Join(w.Dir(), "issues"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") && !current[file.Name()] {
			if err := os.Remove(filepath.Join(w.Dir(), "issues", file.Name())); err != nil {
				return err
			}
		}
	}
	if d.Project != nil && strings.TrimSpace(d.Project.Name) != "" {
		p, err := w.Store.Read(w.ProjectID)
		if err != nil {
			return err
		}
		if p.Repo == "" && p.Name != d.Project.Name {
			p.Name = d.Project.Name
			if err := w.Store.Put(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// SaveCandidate preserves malformed JSON too, so a restarted conversation can
// repair a bad companion response instead of losing it.
func (w Workspace) SaveCandidate(text string) error {
	if _, err := w.Store.Read(w.ProjectID); err != nil {
		return err
	}
	return w.Store.Write(filepath.Join("drafts", w.ProjectID, "candidate.json"), []byte(text))
}

// Checkpoint batches the conversation and draft edits into one response commit.
func (w Workspace) Checkpoint(ctx context.Context) error {
	return w.Store.Checkpoint(ctx, "Planning checkpoint "+w.ProjectID)
}

// Context reloads current specs and recent conversation even when CLI session
// metadata is unavailable on a fresh machine. The full history stays in store.
func (w Workspace) Context() (string, error) {
	d, exists, err := w.Load()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "PLANNING PROJECT %s\n", w.ProjectID)
	p, err := w.Store.Read(w.ProjectID)
	if err != nil {
		return "", err
	}
	if p.Repo == "" {
		b.WriteString("This is a new project with no repository. Decide its name, absolute directory, owner/repo and working branch with the human. Include these as the draft project object. Publishing will create a PRIVATE repository and bootstrap its checkout only after approval. Never create them yourself.\n")
	}
	if exists {
		data, err := json.Marshal(d)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "Current draft package:\n%s\n", data)
	}
	candidate, err := os.ReadFile(filepath.Join(w.Dir(), "candidate.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if len(candidate) > 0 {
		if _, err := Parse(string(candidate)); err != nil {
			fmt.Fprintf(&b, "Latest candidate needs JSON repair:\n%s\n", candidate)
		}
	}
	adoption, err := os.ReadFile(filepath.Join(w.Dir(), "adoption.json"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if len(adoption) > 0 {
		fmt.Fprintf(&b, "Existing-project adoption input:\n%s\nIf respec is true, normalize these issues into the draft and put each superseded issue number on its replacement's supersedes list. If false, plan a fresh milestone without superseding issues.\n", adoption)
	}
	history, err := w.History().Load()
	if err != nil {
		return "", err
	}
	for _, e := range history[max(0, len(history)-12):] {
		fmt.Fprintf(&b, "%s: %s\n", e.Who, e.Text)
	}
	return b.String(), nil
}

// Text is the full reviewable package, including its exact markdown content.
func (d Draft) Text() string {
	var b strings.Builder
	if d.Project != nil {
		fmt.Fprintf(&b, "Create private project: %s\nDirectory: %s\nRepository: %s\nBranch: %s\nBootstrap: one-time clone before publishing\n\n", d.Project.Name, d.Project.Dir, d.Project.Repo, d.Project.Branch)
	}
	fmt.Fprintf(&b, "%s\n\n%s\n", d.Title, d.README)
	for _, is := range d.Issues {
		fmt.Fprintf(&b, "\n--- %s (%s) ---\n%s\n", is.Title, is.ID, is.Text)
	}
	return b.String()
}
