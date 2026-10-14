package app

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/commands"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

func TestArrowsInTheTextFieldNeverMoveTheActivitySelection(t *testing.T) {
	f := newFixture(t)
	f.keys("hello", "up", "down", "left", "left")
	if f.m.view != viewList || f.m.focus != focusInput {
		t.Fatalf("view %d, focus %d after arrows in the text field; want the list and the text field", f.m.view, f.m.focus)
	}
	if got := f.m.input.Position(); got != 3 {
		t.Errorf("text field cursor at %d, want 3 (the arrows belong to the text field)", got)
	}
}

func TestArrowsInTheActivityNeverReachTheTextField(t *testing.T) {
	f := newFixture(t)
	f.keys("half-typed", "tab")
	if f.m.focus != focusActivity {
		t.Fatalf("focus %d after tab, want Activity", f.m.focus)
	}
	f.keys("up", "up", "left", "right", "x")
	if f.m.view != viewSelect || f.m.selected != len(f.m.events)-2 {
		t.Errorf("view %d, selected %d; want the second newest line selected", f.m.view, f.m.selected)
	}
	if got := f.m.input.Value(); got != "half-typed" {
		t.Errorf("text field = %q, want the half-typed text kept and unchanged", got)
	}
	if got := f.m.input.Position(); got != len("half-typed") {
		t.Errorf("text field cursor moved to %d", got)
	}

	// Esc stops selecting, Esc again goes back to typing.
	f.keys("esc")
	if f.m.view != viewList || f.m.focus != focusActivity {
		t.Errorf("after esc: view %d, focus %d", f.m.view, f.m.focus)
	}
	f.keys("esc")
	if f.m.focus != focusInput {
		t.Errorf("after esc twice: focus %d, want the text field", f.m.focus)
	}
}

func TestSlashOpensTheMenuAndFilters(t *testing.T) {
	f := newFixture(t)
	f.keys("/")
	if f.m.focus != focusMenu || f.m.layout().slotRows == 0 {
		t.Fatal("/ did not open the menu")
	}
	f.keys("s")
	if got := names(f.m.menuItems()); !slices.Equal(got, []string{commands.Status, commands.Settings}) {
		t.Errorf("menu after /s = %v", got)
	}
	f.keys("backspace", "backspace")
	if f.m.focus != focusInput || f.m.layout().slotRows != 0 {
		t.Error("deleting the / did not close the menu")
	}

	// ↑/↓ choose, Tab completes, Esc closes and clears.
	f.keys("/", "down", "down", "tab")
	if got := f.m.input.Value(); got != commands.Pause {
		t.Errorf("tab completed to %q, want /pause", got)
	}
	f.keys("esc")
	if f.m.focus != focusInput || f.m.input.Value() != "" {
		t.Errorf("esc: focus %d, text %q", f.m.focus, f.m.input.Value())
	}
}

func TestPastedCommandRuns(t *testing.T) {
	// A paste (or a fast terminal) delivers "/pause" as one key message.
	f := newFixture(t)
	f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/pause")})
	if f.m.focus != focusMenu {
		t.Fatalf("focus %d after pasting /pause, want the menu", f.m.focus)
	}
	f.keys("enter")
	if !f.m.paused {
		t.Error("pasted /pause did not run")
	}
}

func TestSlashFromActivityOpensTheMenu(t *testing.T) {
	f := newFixture(t)
	f.keys("tab")
	f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/status")})
	if f.m.focus != focusMenu || f.m.input.Value() != "/status" {
		t.Fatalf("focus %d, text %q", f.m.focus, f.m.input.Value())
	}
	f.keys("enter")
	if f.m.view != viewStatus {
		t.Errorf("view %d, want /status", f.m.view)
	}
}

func TestMenuShowsOnlyCommandsThatApply(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status loop.Status
		paused bool
		want   []string
	}{
		{"running", loop.Status{Phase: loop.PhaseWorking, Role: session.Dev}, false,
			[]string{commands.Status, commands.Companion, commands.Pause, commands.Log, commands.Settings, commands.Quit}},
		{"paused", loop.Status{Phase: loop.PhasePaused, Role: session.PM}, true,
			[]string{commands.Status, commands.Companion, commands.Resume, commands.Log, commands.Settings, commands.Quit}},
		{"human required", loop.Status{Phase: loop.PhaseHuman}, false,
			[]string{commands.Status, commands.Companion, commands.Log, commands.Settings, commands.Quit}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.paused {
				f.keys("/pause", "enter")
			}
			f.send(StatusMsg(tt.status))
			f.keys("/")
			if got := names(f.m.menuItems()); !slices.Equal(got, tt.want) {
				t.Errorf("menu = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnterOnAnActivityLineOpensTheLogThere(t *testing.T) {
	f := newFixture(t)
	f.keys("tab", "up", "up", "up") // "$ git push ...", which points at its "exec" line
	if !strings.HasPrefix(f.m.events[f.m.selected].Text, "$ git push") {
		t.Fatalf("selected %q", f.m.events[f.m.selected].Text)
	}
	f.keys("enter")
	if f.m.view != viewLog {
		t.Fatalf("view %d after enter, want the log", f.m.view)
	}
	target := f.m.log.lines[f.m.log.target]
	if target.stamp != "22:52:07" || target.text != "exec" {
		t.Errorf("log opened at %+v, want the 22:52:07 exec line", target)
	}
	if f.m.log.path() != f.logPath || len(f.m.log.runs) != 1 {
		t.Errorf("log %s with runs %+v; want only this repo's run", f.m.log.path(), f.m.log.runs)
	}
	// The line jumped to is on screen.
	if row := f.m.log.targetRow() - f.m.log.vp.YOffset; row < 0 || row >= f.m.log.vp.Height {
		t.Errorf("target on row %d of the view", row)
	}

	// Huddle's own lines have no log: Enter does nothing.
	f.keys("esc", "up", "up", "up", "up", "up", "up", "up", "up", "up", "up", "enter")
	if f.m.view != viewSelect || !f.m.events[f.m.selected].LogRef.IsZero() {
		t.Errorf("view %d on %q", f.m.view, f.m.events[f.m.selected].Text)
	}
}

func TestLogKeys(t *testing.T) {
	f := newFixture(t)
	f.keys("/log", "enter")
	if f.m.view != viewLog || f.m.focus != focusActivity || f.m.log.path() != f.logPath {
		t.Fatalf("view %d, focus %d, log %q", f.m.view, f.m.focus, f.m.log.path())
	}
	f.keys("g")
	if f.m.log.vp.YOffset != 0 {
		t.Errorf("g: offset %d, want the top", f.m.log.vp.YOffset)
	}
	f.keys("G")
	if !f.m.log.vp.AtBottom() {
		t.Error("G: not at the end")
	}
	f.keys("[", "]") // a single run: nothing to switch to
	if f.m.log.run != 0 {
		t.Errorf("run %d", f.m.log.run)
	}
	f.keys("esc")
	if f.m.view != viewList {
		t.Errorf("esc: view %d, want the list", f.m.view)
	}
}

func TestPauseAndResume(t *testing.T) {
	f := newFixture(t)
	f.keys("/pause", "enter")
	if !f.m.cfg.Controls.Paused() || !f.m.paused {
		t.Fatal("/pause did not pause the loop")
	}
	if f.stopped {
		t.Error("/pause stopped the loop; the turn must finish normally")
	}
	if !strings.Contains(f.lastEvent(), "current turn will finish") {
		t.Errorf("last event %q", f.lastEvent())
	}
	f.keys("/resume", "enter")
	if f.m.cfg.Controls.Paused() || f.m.paused {
		t.Error("/resume did not resume the loop")
	}
}

func TestQuitStopsTheLoopThenExits(t *testing.T) {
	for _, key := range []string{"/quit", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			f := newFixture(t)
			var cmd tea.Cmd
			if key == "ctrl+c" {
				cmd = f.send(tea.KeyMsg{Type: tea.KeyCtrlC})
			} else {
				f.keys("/quit")
				cmd = f.send(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if !f.stopped || !f.m.quitting {
				t.Fatal("the loop was not stopped")
			}
			if cmd != nil {
				t.Error("the screen closed before the loop put its label back")
			}
			// The loop has restored the label and ended.
			cmd = f.send(LoopDoneMsg{Code: loop.ExitInterrupted})
			if cmd == nil || cmd() != tea.Quit() {
				t.Error("the screen did not close after the loop ended")
			}
		})
	}
}

func TestMouseWheelScrollsThePaneUnderThePointer(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 60; i++ {
		f.send(EventMsg(activity.Event{Time: at(23, 0), Actor: "codex", Text: "more work"}))
	}
	bottom := f.m.list.YOffset
	if bottom == 0 || !f.m.list.AtBottom() {
		t.Fatalf("list offset %d; want it following the newest event", bottom)
	}

	wheel := func(y int, b tea.MouseButton) {
		f.send(tea.MouseMsg{X: 10, Y: y, Button: b, Action: tea.MouseActionPress})
	}
	wheel(5, tea.MouseButtonWheelUp) // over the Activity pane
	if f.m.list.YOffset != bottom-wheelRows {
		t.Errorf("wheel up over Activity: offset %d, want %d", f.m.list.YOffset, bottom-wheelRows)
	}
	wheel(testHeight-2, tea.MouseButtonWheelDown) // over the text field
	if f.m.list.YOffset != bottom-wheelRows {
		t.Errorf("wheel over the text field scrolled the Activity to %d", f.m.list.YOffset)
	}

	// Scrolled up, the list stays put when new events arrive.
	f.send(EventMsg(activity.Event{Time: at(23, 1), Actor: "codex", Text: "newest"}))
	if f.m.list.YOffset != bottom-wheelRows {
		t.Errorf("a new event moved the scrolled-up list to %d", f.m.list.YOffset)
	}

	// The wheel over the open menu moves its selection.
	f.keys("/")
	l := f.m.layout()
	wheel(l.slotTop+2, tea.MouseButtonWheelDown)
	if f.m.menuSel != 1 {
		t.Errorf("wheel over the menu: selection %d, want 1", f.m.menuSel)
	}
}

func TestClickFocusesActivityAndSelectsALine(t *testing.T) {
	f := newFixture(t)
	// Row 2 is the Activity border, row 3 its title, row 4 the first event.
	f.send(tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if f.m.focus != focusActivity || f.m.view != viewSelect || f.m.selected != 1 {
		t.Errorf("focus %d, view %d, selected %d; want Activity with the second event selected", f.m.focus, f.m.view, f.m.selected)
	}
	f.send(tea.MouseMsg{X: 10, Y: testHeight - 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if f.m.focus != focusInput {
		t.Errorf("click on the text field: focus %d", f.m.focus)
	}
}

func TestFactsRefreshAfterATurnAndEveryMinute(t *testing.T) {
	f := newFixture(t)
	// The tests never run a tick's command, which would wait for the timer.
	// They run the fetch it asks for by hand.
	f.run(f.m.fetchFacts())
	fetches := f.gh.FactsRequests

	// The Dev's turn ends: the facts are refreshed straight away.
	f.run(f.send(StatusMsg{Phase: loop.PhaseWorking, Role: session.PM, Turn: 6}))
	if f.gh.FactsRequests != fetches {
		t.Error("fetched while the turn went on")
	}
	f.run(f.send(StatusMsg{Phase: loop.PhaseHuman, Turn: 6}))
	if f.gh.FactsRequests != fetches+1 || f.m.fetching {
		t.Errorf("%d fetches after a turn, want 1", f.gh.FactsRequests-fetches)
	}

	// Ticks fetch only once a minute has passed since the last fetch.
	f.now = f.now.Add(factsInterval / 2)
	f.send(tickMsg(f.now))
	if f.m.fetching {
		t.Error("a tick fetched within the minute")
	}
	f.now = f.now.Add(factsInterval)
	f.send(tickMsg(f.now))
	if !f.m.fetching {
		t.Error("no fetch a minute later")
	}
}

func names(cmds []commands.Command) []string {
	var out []string
	for _, c := range cmds {
		out = append(out, c.Name)
	}
	return out
}

func TestWordsThatAreKeyNamesAreTyped(t *testing.T) {
	f := newFixture(t)
	// Fast input arrives as one message per word.
	for _, w := range []string{"turn", " ", "right", " ", "then", " ", "up"} {
		f.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(w)})
	}
	if got := f.m.input.Value(); got != "turn right then up" {
		t.Errorf("text field = %q", got)
	}
}
