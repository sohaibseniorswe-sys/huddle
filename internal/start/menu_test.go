package start

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// update rewrites the golden files: go test ./internal/start -update
var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// TestMain draws every view without colours, so golden files are plain
// text.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// screenFacts are the facts of screens 14 and 15.
func screenFacts() Facts {
	return Facts{
		Repo: "sohaibseniorswe-sys/palantir",
		Dir:  "~/projects/palantir",
		Milestones: []MilestoneEntry{
			{Title: "Milestone 4 · Maps and exports", Issues: 6},
			{Title: "Milestone 5 · Admin dashboard", Issues: 4, Closed: 1, Control: 81},
		},
		Resume: &gh.Issue{Number: 58, Milestone: "Milestone 3", Labels: []string{"control", gh.StateDevWorking}},
	}
}

// send passes msgs to m's Update one after another.
func send(m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

var (
	size  = tea.WindowSizeMsg{Width: 100, Height: 34}
	down  = tea.KeyMsg{Type: tea.KeyDown}
	up    = tea.KeyMsg{Type: tea.KeyUp}
	enter = tea.KeyMsg{Type: tea.KeyEnter}
	esc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func typeText(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// golden compares got with testdata/name.golden, or rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/start -update)", err)
	}
	if got != string(want) {
		t.Errorf("%s differs from the golden file:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
	if h := lipgloss.Height(got); h != 34 {
		t.Errorf("%s is %d rows high, want 34", name, h)
	}
}

func TestScreenGoldens(t *testing.T) {
	m, _ := send(New(screenFacts(), "Settings text"), size)
	golden(t, "14-start-menu", m.View())

	m, _ = send(m, down, enter)
	golden(t, "15-pick-milestone", m.View())

	m, _ = send(m, enter, typeText("Start with the export issue"))
	golden(t, "15b-first-message", m.View())
}

func TestMenuShowsOnlyWhatMakesSense(t *testing.T) {
	m, _ := send(New(Facts{Repo: "o/r", Dir: "~/r"}, ""), size)
	var names []string
	for _, o := range m.options() {
		names = append(names, o.name)
	}
	want := []string{"Plan new milestones with the companion", "Settings", "Quit"}
	if len(names) != len(want) {
		t.Fatalf("options = %q, want %q", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("option %d = %q, want %q", i, names[i], want[i])
		}
	}
}

func TestMenuChoices(t *testing.T) {
	cases := []struct {
		name string
		keys []tea.Msg
		want Choice
	}{
		{"plan", []tea.Msg{enter}, Choice{Kind: KindPlan}},
		{"resume", []tea.Msg{down, down, enter}, Choice{Kind: KindResume}},
		{"quit row", []tea.Msg{down, down, down, down, enter}, Choice{Kind: KindQuit}},
		{"q", []tea.Msg{typeText("q")}, Choice{Kind: KindQuit}},
		{"esc", []tea.Msg{esc}, Choice{Kind: KindQuit}},
		{"ctrl+c in the message", []tea.Msg{down, enter, enter, tea.KeyMsg{Type: tea.KeyCtrlC}}, Choice{Kind: KindQuit}},
		{"run with the default prompt", []tea.Msg{down, enter, enter, enter},
			Choice{Kind: KindRun, Milestone: screenFacts().Milestones[0]}},
		{"run the second with a message", []tea.Msg{down, enter, down, enter, typeText("Do #3 first"), enter},
			Choice{Kind: KindRun, Milestone: screenFacts().Milestones[1], Message: "Do #3 first"}},
		{"esc from the message goes back, not out", []tea.Msg{down, enter, enter, esc, esc, up, enter},
			Choice{Kind: KindPlan}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, cmd := send(New(screenFacts(), ""), append([]tea.Msg{size}, tc.keys...)...)
			got := m.Choice()
			if got.Kind != tc.want.Kind || got.Milestone != tc.want.Milestone || got.Message != tc.want.Message {
				t.Errorf("choice = %+v, want %+v", got, tc.want)
			}
			if tc.want.Kind == KindResume && got.Resume.Number != 58 {
				t.Errorf("resume = #%d, want #58", got.Resume.Number)
			}
			if cmd == nil {
				t.Fatal("no command: the menu did not quit")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Error("the last command is not tea.Quit")
			}
		})
	}
}

func TestSettingsPage(t *testing.T) {
	m, _ := send(New(screenFacts(), "Repo  o/r"), size, down, down, down, enter)
	if m.page != pageSettings {
		t.Fatalf("page = %d, want settings", m.page)
	}
	m, _ = send(m, esc)
	if m.page != pageMenu || m.sel != 3 {
		t.Errorf("after esc: page %d, row %d; want the menu at Settings", m.page, m.sel)
	}
}
