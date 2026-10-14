package companion

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Who wrote a history entry.
const (
	WhoYou       = "you"       // the human
	WhoCompanion = "companion" // the companion's reply, without its tags
	WhoHuddle    = "huddle"    // a note from Huddle, such as an error
	WhoDecision  = "decision"  // a decision the human agreed and Huddle posted
)

// Entry is one line of the conversation.
type Entry struct {
	Time   time.Time `json:"time"`
	Who    string    `json:"who"` // one of the Who constants
	Text   string    `json:"text"`
	Reason string    `json:"reason,omitempty"` // WhoDecision only
}

// History is the conversation's file on disk, one JSON entry per line
// (JSON Lines): <home>/.huddle/companion/<owner>_<repo>_<issue>.jsonl.
// Appending a line never rewrites what is already there, so a crash can
// lose at most the entry being written.
type History struct {
	Path string
}

// NewHistory returns the History of control issue number issue on repo
// ("owner/name").
func NewHistory(home, repo string, issue int) History {
	name := fmt.Sprintf("%s_%d.jsonl", strings.ReplaceAll(repo, "/", "_"), issue)
	return History{Path: filepath.Join(home, ".huddle", "companion", name)}
}

// Load returns every entry, oldest first. A missing file is an empty
// conversation. A line that is not valid JSON (the end of an interrupted
// write) is skipped.
func (h History) Load() ([]Entry, error) {
	f, err := os.Open(h.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read companion history: %w", err)
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024) // allow long replies
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue // a torn line; see above
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		return entries, fmt.Errorf("read companion history: %w", err)
	}
	return entries, nil
}

// Append adds e at the end of the file, creating it if needed.
func (h History) Append(e Entry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode companion history: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(h.Path), 0o755); err != nil {
		return fmt.Errorf("create companion history dir: %w", err)
	}
	f, err := os.OpenFile(h.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open companion history: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return fmt.Errorf("write companion history: %w", err)
	}
	return f.Close()
}
