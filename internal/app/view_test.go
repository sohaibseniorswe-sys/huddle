package app

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// update rewrites the golden files: go test ./internal/app -update
var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// checkGolden compares view with testdata/<name>.golden.
func checkGolden(t *testing.T, name, view string) {
	t.Helper()
	if h := lipgloss.Height(view); h != testHeight {
		t.Errorf("%s: %d rows, want %d", name, h, testHeight)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w != testWidth {
			t.Errorf("%s: row %d is %d wide, want %d", name, i, w, testWidth)
		}
	}
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(view), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/app -update to create it)", err)
	}
	if view != string(want) {
		t.Errorf("%s differs from %s (go test ./internal/app -update rewrites it):\n%s", name, path, view)
	}
}

// The screens below match docs/v3/screens at 100 × 34, without the
// Companion pane, which arrives in v3.2.

func TestViewList(t *testing.T) { // screen 01
	f := newFixture(t)
	checkGolden(t, "01-list", f.m.View())
}

func TestViewMenu(t *testing.T) { // screen 03
	f := newFixture(t)
	f.keys("/")
	checkGolden(t, "03-menu", f.m.View())
}

func TestViewMenuFiltered(t *testing.T) { // screen 04
	f := newFixture(t)
	f.keys("/s")
	checkGolden(t, "04-menu-filter", f.m.View())
}

func TestViewPaused(t *testing.T) { // screen 05
	f := newFixture(t)
	f.keys("/pause", "enter")
	f.send(EventMsg(f.m.huddleEvent("Dev (Codex) set state:pm-ready")))
	f.send(StatusMsg{Phase: loop.PhasePaused, Role: session.PM, Turn: 5, Since: f.now})
	f.send(EventMsg(f.m.huddleEvent("Paused. The PM (Claude Code) is next and starts when you /resume")))
	f.keys("/")
	checkGolden(t, "05-paused", f.m.View())
}

func TestViewStatus(t *testing.T) { // screen 06
	f := newFixture(t)
	f.keys("/status", "enter")
	checkGolden(t, "06-status", f.m.View())
}

func TestViewSelect(t *testing.T) { // screen 07
	f := newFixture(t)
	f.keys("tab", "up", "up")
	checkGolden(t, "07-select", f.m.View())
}

func TestViewLog(t *testing.T) { // screen 08
	f := newFixture(t)
	f.keys("tab", "up", "up", "up", "enter")
	checkGolden(t, "08-log", f.m.View())
}

func TestViewTooSmall(t *testing.T) {
	f := newFixture(t)
	f.send(sizeMsg(79, 34))
	if got := f.m.View(); got != tooSmallText {
		t.Errorf("View() at 79 × 34 = %q", got)
	}
	f.send(sizeMsg(80, 23))
	if got := f.m.View(); got != tooSmallText {
		t.Errorf("View() at 80 × 23 = %q", got)
	}
	f.send(sizeMsg(80, 24))
	if got := f.m.View(); lipgloss.Height(got) != 24 {
		t.Errorf("View() at 80 × 24 has %d rows", lipgloss.Height(got))
	}
}
