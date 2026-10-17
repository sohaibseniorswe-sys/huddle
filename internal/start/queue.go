package start

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// Queue is the milestone queue: the control issues of agreed plans, in
// the order they run. It is a JSON file,
// <root>/queue/<owner>_<repo>.json, holding a list of control
// issue numbers, so it survives a restart of Huddle.
//
// The first number is the milestone that runs now (or runs next, if
// Huddle was stopped). When a milestone reaches state:done, the run screen
// removes it and starts the new first one.
type Queue struct {
	Path string
}

// NewQueue returns the queue of repo ("owner/name").
func NewQueue(home, repo string) Queue {
	name := strings.ReplaceAll(repo, "/", "_") + ".json"
	return Queue{Path: filepath.Join(home, "queue", name)}
}

// queueFile is the file's JSON.
type queueFile struct {
	Controls []int `json:"controls"`
}

// Load returns the queued control issue numbers, first to run first. A
// missing file is an empty queue.
func (q Queue) Load() ([]int, error) {
	data, err := os.ReadFile(q.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the milestone queue: %w", err)
	}
	var f queueFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("read the milestone queue %s: %w", q.Path, err)
	}
	return f.Controls, nil
}

// Add puts controls at the end of the queue, in order. A number already
// queued is not added twice.
func (q Queue) Add(controls ...int) error {
	queued, err := q.Load()
	if err != nil {
		return err
	}
	for _, c := range controls {
		if !slices.Contains(queued, c) {
			queued = append(queued, c)
		}
	}
	return q.save(queued)
}

// Remove takes control out of the queue, wherever it is. Removing a
// number that is not queued is not an error.
func (q Queue) Remove(control int) error {
	queued, err := q.Load()
	if err != nil {
		return err
	}
	if !slices.Contains(queued, control) {
		return nil
	}
	return q.save(slices.DeleteFunc(queued, func(c int) bool { return c == control }))
}

// save writes the queue: to a temporary file first, then renamed over the
// old one, so an interrupted write never leaves half a file.
func (q Queue) save(controls []int) error {
	if controls == nil {
		controls = []int{} // "[]" rather than "null" in the file
	}
	data, err := json.MarshalIndent(queueFile{Controls: controls}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(q.Path), 0o755); err != nil {
		return fmt.Errorf("create the queue folder: %w", err)
	}
	tmp := q.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write the milestone queue: %w", err)
	}
	if err := os.Rename(tmp, q.Path); err != nil {
		return fmt.Errorf("write the milestone queue: %w", err)
	}
	return nil
}

// Next returns the first queued control issue that is still open, ready
// to run: if it has no state label yet, Next sets state:dev-ready, which
// starts the milestone. Queued issues that are closed or gone are taken
// out of the queue on the way. ok is false when nothing is left.
func (q Queue) Next(ctx context.Context, client gh.Client, repo string) (is gh.Issue, ok bool, err error) {
	queued, err := q.Load()
	if err != nil || len(queued) == 0 {
		return gh.Issue{}, false, err
	}
	open, err := client.ListIssues(ctx, repo, "open")
	if err != nil {
		return gh.Issue{}, false, fmt.Errorf("list the open issues of %s: %w", repo, err)
	}
	for _, n := range queued {
		i := slices.IndexFunc(open, func(is gh.Issue) bool { return is.Number == n })
		if i < 0 {
			if err := q.Remove(n); err != nil {
				return gh.Issue{}, false, err
			}
			continue // closed by hand, or deleted: skip it
		}
		is := open[i]
		if ResumeState(is) == "" {
			c := gh.Control{Client: client, Repo: repo, Number: n}
			if err := c.SetState(ctx, gh.StateDevReady); err != nil {
				return gh.Issue{}, false, err
			}
		}
		return is, true, nil
	}
	return gh.Issue{}, false, nil
}
