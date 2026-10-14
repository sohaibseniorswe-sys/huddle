package app

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
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

// The screens below match docs/v3/screens at 100 × 34.

func TestViewList(t *testing.T) { // screen 01
	f := newFixture(t)
	checkGolden(t, "01-list", f.m.View())
}

func TestViewCompanion(t *testing.T) { // screen 02
	f := newFixture(t)
	f.talk()
	f.keys("Not now. Put CSV in Milestone 4.", "tab", "tab")
	checkGolden(t, "02-companion", f.m.View())
}

func TestViewCompanionThinking(t *testing.T) {
	f := newFixture(t)
	f.talk()
	// Update starts the companion's run, but the test does not run it.
	f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Why?")})
	f.send(tea.KeyMsg{Type: tea.KeyEnter})
	checkGolden(t, "02b-companion-thinking", f.m.View())
}

func TestViewMenu(t *testing.T) { // screen 03: ¼ Activity, ½ Companion, ¼ menu
	f := newFixture(t)
	f.talk()
	f.keys("/")
	checkGolden(t, "03-menu", f.m.View())
}

func TestViewMenuWithoutCompanion(t *testing.T) {
	f := newFixture(t)
	f.keys("/")
	checkGolden(t, "03b-menu-no-companion", f.m.View())
}

func TestViewMenuFiltered(t *testing.T) { // screen 04
	f := newFixture(t)
	f.talk()
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
	f.talk()
	f.keys("/status", "enter")
	checkGolden(t, "06-status", f.m.View())
}

func TestViewSelect(t *testing.T) { // screen 07
	f := newFixture(t)
	f.talk()
	f.keys("tab", "up", "up")
	checkGolden(t, "07-select", f.m.View())
}

func TestViewLog(t *testing.T) { // screen 08
	f := newFixture(t)
	f.keys("tab", "up", "up", "up", "enter")
	f.comp.replies = []string{"Codex's raw log for turn 5 at 22:52:07. It pushed agent/issue-62, opened PR #73 for the /export endpoint and started watching CI, which passed 36 seconds later at 22:52:43."}
	f.keys("tab", "What am I looking at?", "enter", "tab")
	checkGolden(t, "08-log", f.m.View())
}

func TestViewLogWithoutCompanion(t *testing.T) {
	f := newFixture(t)
	f.keys("tab", "up", "up", "up", "enter")
	checkGolden(t, "08b-log-no-companion", f.m.View())
}

func TestViewDecision(t *testing.T) { // screen 10: the text field keeps the keys
	f := newDecisionFixture(t)
	checkGolden(t, "10-decision", f.m.View())
}

func TestViewDecisionFocused(t *testing.T) { // screen 10b: after /decision
	f := newDecisionFixture(t)
	f.keys("tab", "/decision", "enter")
	checkGolden(t, "10b-decision-focused", f.m.View())
}

func TestViewActionCard(t *testing.T) {
	f := newFixture(t)
	f.comp.replies = []string{"CI on main is red since 22:41. Pausing lets you look before the PM starts. <action>pause</action>"}
	f.keys("Should we stop?", "enter")
	checkGolden(t, "11-action", f.m.View())
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

func TestViewSettings(t *testing.T) { // screen 09
	f := newSettingsFixture(t)
	f.keys("/settings", "enter")
	if f.m.view != viewSettings || f.m.focus != focusActivity {
		t.Fatalf("view %d, focus %d after /settings", f.m.view, f.m.focus)
	}
	checkGolden(t, "09-settings", f.m.View())

	f.keys("esc")
	if f.m.view != viewList {
		t.Errorf("view %d after esc, want the Activity list", f.m.view)
	}
}

func TestViewSwappedRoles(t *testing.T) {
	f := newFixture(t)
	f.m.cfg.Team = agents.Team{
		Dev:       agents.Choice{CLI: agents.Claude()},
		PM:        agents.Choice{CLI: agents.Codex()},
		Companion: agents.Choice{CLI: agents.Codex()},
	}
	f.send(StatusMsg{Phase: loop.PhaseWorking, Role: session.Dev, Turn: 6, Since: at(22, 50)})
	view := ansi.Strip(f.m.View())
	for _, want := range []string{"Dev (Claude) working", "Usage Claude ok · Codex ok"} {
		if !strings.Contains(view, want) {
			t.Errorf("screen lacks %q:\n%s", want, view)
		}
	}
	// A usage wait shows on the CLI that plays the waiting role.
	f.send(StatusMsg{Phase: loop.PhaseUsageWait, Role: session.PM, Turn: 6, Since: at(22, 55), Until: at(23, 45)})
	if got := f.m.usage(true); got != "Claude Code ok · Codex waiting until 23:45" {
		t.Errorf("usage = %q", got)
	}
	// One CLI playing both roles is named once.
	f.m.cfg.Team.PM = agents.Choice{CLI: agents.Claude()}
	if got := f.m.usage(false); got != "Claude waiting until 23:45" {
		t.Errorf("usage with Claude Code in both roles = %q", got)
	}
}
