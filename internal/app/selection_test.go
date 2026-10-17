package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

func TestSelectionPlainTextAndStyles(t *testing.T) {
	content := "\x1b[31malpha\x1b[0m\nβeta 界\nlast"
	for _, tt := range []struct {
		name string
		a, b textPoint
		want string
	}{
		{"forward", textPoint{0, 1}, textPoint{1, 3}, "lpha\nβeta"},
		{"backward", textPoint{1, 3}, textPoint{0, 1}, "lpha\nβeta"},
		{"Unicode cells", textPoint{1, 5}, textPoint{1, 6}, "界"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := textSelection{pane: selectLog, key: "source", anchor: tt.a, end: tt.b}
			if got := s.plainText(content); got != tt.want {
				t.Fatalf("copy %q want %q", got, tt.want)
			}
			if got := s.highlight(content, "other"); got != content {
				t.Fatal("highlighted another source")
			}
		})
	}
}

func selectionFixture(t *testing.T) *fixture {
	f := newFixture(t)
	f.m.view = viewLog
	f.m.focus = focusActivity
	f.m.log.runs = []logRun{{path: "sandbox-selection.log", agent: "Codex", turn: 1}}
	f.m.log.run = 0
	f.m.log.from = 0
	f.m.log.to = 500
	f.m.log.size = 500
	f.m.log.target = -1
	for i := 0; i < 80; i++ {
		f.m.log.lines = append(f.m.log.lines, logLine{stamp: "12:00:00", text: fmt.Sprintf("sandbox output row %02d", i)})
	}
	f.m = f.m.sync()
	f.m.log.vp.GotoTop()
	return f
}

func TestDragSelectionAcrossScrollAndCopy(t *testing.T) {
	f := selectionFixture(t)
	top := f.m.layout().activityTop + 2
	f.send(tea.MouseMsg{X: 12, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	anchor := f.m.selection.anchor
	f.send(tea.MouseMsg{X: 30, Y: top + f.m.log.vp.Height + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	for i := 0; i < 6; i++ {
		f.send(selectionTickMsg{})
	}
	if f.m.log.vp.YOffset == 0 || f.m.selection.anchor != anchor || f.m.selection.end.row <= anchor.row {
		t.Fatalf("selection did not survive scroll: %+v offset %d", f.m.selection, f.m.log.vp.YOffset)
	}
	f.send(tea.MouseMsg{X: 30, Y: top + f.m.log.vp.Height + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if f.m.selection.dragging || !f.m.selection.active() {
		t.Fatal("release lost selection")
	}
	before := f.m.selection
	f.send(tea.KeyMsg{Type: tea.KeyPgUp})
	if f.m.selection.anchor != before.anchor || f.m.selection.end != before.end {
		t.Fatal("keyboard scroll changed selection")
	}
	f.send(tea.KeyMsg{Type: tea.KeyCtrlC})
	if f.m.quitting {
		t.Fatal("copy interrupted the build")
	}
	frame := f.m.View()
	prefix := "\x1b]52;c;"
	a := strings.Index(frame, prefix)
	if a < 0 {
		t.Fatal("copy did not reach the renderer")
	}
	end := strings.IndexByte(frame[a+len(prefix):], '\a')
	if end < 0 {
		t.Fatal("invalid OSC 52")
	}
	copied, err := base64.StdEncoding.DecodeString(frame[a+len(prefix) : a+len(prefix)+end])
	if err != nil || !strings.Contains(string(copied), "sandbox output row 00") || !strings.Contains(string(copied), "sandbox output row 20") {
		t.Fatalf("clipboard content %q %v", copied, err)
	}
	f.send(tea.KeyMsg{Type: tea.KeyEsc})
	if f.m.selection.active() {
		t.Fatal("Escape did not clear selection")
	}
}

func TestSelectionScrollsUpAndClearsOnSourceChange(t *testing.T) {
	f := selectionFixture(t)
	f.m.log.vp.SetYOffset(20)
	top := f.m.layout().activityTop + 2
	f.send(tea.MouseMsg{X: 12, Y: top + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	f.send(tea.MouseMsg{X: 30, Y: top - 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	f.send(selectionTickMsg{})
	if f.m.log.vp.YOffset >= 20 || f.m.selection.end.row >= f.m.selection.anchor.row {
		t.Fatal("upward drag did not scroll/select")
	}
	f.send(tea.WindowSizeMsg{Width: 110, Height: 34})
	if f.m.selection.active() {
		t.Fatal("resize retained invalid coordinates")
	}
}

func TestCompanionSelectionDoesNotFollowNewOutput(t *testing.T) {
	f := newFixture(t)
	f.talk()
	f.m = f.m.sync()
	top := f.m.layout().companionTop + 2
	f.send(tea.MouseMsg{X: 4, Y: top, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	f.send(tea.MouseMsg{X: 30, Y: top + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	f.send(tea.MouseMsg{X: 30, Y: top + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if f.m.selection.pane != selectChat {
		t.Fatal("companion text not selectable")
	}
	offset := f.m.chatVP.YOffset
	f.m = f.m.sync()
	if f.m.chatVP.YOffset != offset {
		t.Fatal("selected conversation jumped to bottom")
	}
	f.send(tea.KeyMsg{Type: tea.KeyCtrlY})
	if !strings.HasPrefix(f.m.View(), "\x1b]52;c;") {
		t.Fatal("companion copy missing")
	}
}

// TestSelectionTerminalDemo is an opt-in terminal fixture for manual mouse and
// clipboard verification. It has no build loop, real agents or GitHub writes.
func TestSelectionTerminalDemo(t *testing.T) {
	if os.Getenv("HUDDLE_SELECTION_DEMO") != "1" {
		t.Skip("interactive terminal verification fixture")
	}
	t.Setenv("HUDDLE_HOME", t.TempDir())
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	cfg := Config{Repo: "sohaibseniorswe-sys/huddle-sandbox", Planning: true, Now: time.Now, Team: agents.Team{Dev: agents.Choice{CLI: agents.Codex()}, PM: agents.Choice{CLI: agents.Claude()}, Companion: agents.Choice{CLI: agents.Claude()}}, Control: gh.Control{Client: &gh.Fake{}, Repo: "sohaibseniorswe-sys/huddle-sandbox"}}
	m := New(cfg)
	m.loopDone = true
	m.view = viewLog
	m.focus = focusActivity
	m.width = 100
	m.height = 34
	m.log.runs = []logRun{{path: "isolated-sandbox-demo", agent: "Codex", turn: 1}}
	m.log.run = 0
	m.log.target = -1
	for i := 0; i < 100; i++ {
		m.log.lines = append(m.log.lines, logLine{stamp: "12:00:00", text: fmt.Sprintf("sandbox selectable output row %02d", i)})
	}
	m = m.sync()
	m.log.vp.GotoTop()
	p := tea.NewProgram(m, tea.WithInput(os.Stdin), tea.WithOutput(os.Stdout), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
}
