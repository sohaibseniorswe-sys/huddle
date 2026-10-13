// Package session keeps each agent's session id per control issue on disk,
// so a restarted Huddle resumes the same agent sessions (section 8.4).
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Role names an agent.
type Role string

const (
	Dev Role = "dev"
	PM  Role = "pm"
)

// IDs are the saved session ids; "" means none.
type IDs struct {
	Dev string `json:"dev,omitempty"`
	PM  string `json:"pm,omitempty"`
}

// Get returns the id saved for role.
func (ids IDs) Get(role Role) string {
	if role == PM {
		return ids.PM
	}
	return ids.Dev
}

func (ids *IDs) set(role Role, id string) {
	if role == PM {
		ids.PM = id
	} else {
		ids.Dev = id
	}
}

// Store is the session file for one control issue:
// <home>/.huddle/sessions/<owner>_<repo>_<control-issue>.json.
type Store struct {
	Path string
}

// New returns the Store for control issue number issue on repo ("owner/name").
func New(home, repo string, issue int) Store {
	name := fmt.Sprintf("%s_%d.json", strings.ReplaceAll(repo, "/", "_"), issue)
	return Store{Path: filepath.Join(home, ".huddle", "sessions", name)}
}

// Load returns the saved ids. A missing or unreadable file means no sessions.
func (s Store) Load() IDs {
	var ids IDs
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return IDs{}
	}
	if err := json.Unmarshal(data, &ids); err != nil {
		return IDs{}
	}
	return ids
}

// Save records id as role's session, keeping the other role's.
func (s Store) Save(role Role, id string) error {
	ids := s.Load()
	ids.set(role, id)
	return s.write(ids)
}

// Forget removes role's session, keeping the other role's.
func (s Store) Forget(role Role) error {
	return s.Save(role, "")
}

func (s Store) write(ids IDs) error {
	data, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("create sessions dir: %w", err)
	}
	// Write and rename, so an interrupted write never leaves a corrupt file.
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}
