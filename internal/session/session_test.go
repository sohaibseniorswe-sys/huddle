package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewPath(t *testing.T) {
	got := New("/home/u", "sohaibseniorswe-sys/palantir", 58).Path
	if want := "/home/u/.huddle/sessions/sohaibseniorswe-sys_palantir_58.json"; got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	s := New(t.TempDir(), "o/r", 3)
	if err := s.Save(Dev, "codex-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(PM, "claude-1"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Load(), (IDs{Dev: "codex-1", PM: "claude-1"}); got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
	// A fresh session's id replaces the saved one.
	if err := s.Save(Dev, "codex-2"); err != nil {
		t.Fatal(err)
	}
	ids := New(filepath.Dir(filepath.Dir(filepath.Dir(s.Path))), "o/r", 3).Load()
	if ids.Get(Dev) != "codex-2" || ids.Get(PM) != "claude-1" {
		t.Errorf("Load() after replace = %+v", ids)
	}
	data, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "{\n  \"dev\": \"codex-2\",\n  \"pm\": \"claude-1\"\n}\n"; string(data) != want {
		t.Errorf("file = %q, want %q", data, want)
	}
}

func TestMissingFile(t *testing.T) {
	if got := New(t.TempDir(), "o/r", 1).Load(); got != (IDs{}) {
		t.Errorf("Load() = %+v, want no sessions", got)
	}
}

func TestCorruptFile(t *testing.T) {
	s := New(t.TempDir(), "o/r", 1)
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(); got != (IDs{}) {
		t.Errorf("Load() = %+v, want no sessions", got)
	}
	// Saving over a corrupt file works and starts from no sessions.
	if err := s.Save(PM, "claude-1"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Load(), (IDs{PM: "claude-1"}); got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestUnreadableFile(t *testing.T) {
	s := New(t.TempDir(), "o/r", 1)
	// A directory where the file should be cannot be read as a file.
	if err := os.MkdirAll(s.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := s.Load(); got != (IDs{}) {
		t.Errorf("Load() = %+v, want no sessions", got)
	}
}

func TestForget(t *testing.T) {
	s := New(t.TempDir(), "o/r", 1)
	if err := s.Save(Dev, "codex-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(PM, "claude-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Forget(Dev); err != nil {
		t.Fatal(err)
	}
	if got, want := s.Load(), (IDs{PM: "claude-1"}); got != want {
		t.Errorf("Load() after Forget(Dev) = %+v, want %+v", got, want)
	}
	// Forgetting with no file is fine.
	if err := New(t.TempDir(), "o/r", 2).Forget(PM); err != nil {
		t.Errorf("Forget() with no file: %v", err)
	}
}

func TestCompanionHasItsOwnFile(t *testing.T) {
	home := t.TempDir()
	c := NewCompanion(home, "sohaibseniorswe-sys/palantir", 58)
	if want := filepath.Join(home, ".huddle/sessions/sohaibseniorswe-sys_palantir_58_companion.json"); c.Path != want {
		t.Errorf("Path = %q, want %q", c.Path, want)
	}
	if err := c.Save(Companion, "talk-1"); err != nil {
		t.Fatal(err)
	}
	if got := c.Load().Get(Companion); got != "talk-1" {
		t.Errorf("Get(Companion) = %q, want talk-1", got)
	}
	// The Dev and PM file is untouched.
	if got := New(home, "sohaibseniorswe-sys/palantir", 58).Load(); got != (IDs{}) {
		t.Errorf("Dev and PM ids = %+v, want none", got)
	}
}
