package wizard

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// update rewrites the golden files: go test ./internal/wizard -update
var update = flag.Bool("update", false, "rewrite the golden files in testdata")

const (
	testWidth  = 100
	testHeight = 34
	testHome   = "/home/sweeng"
	testRepo   = "sohaibseniorswe-sys/palantir"
)

// TestMain draws every view without colours, so golden files are plain
// text.
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

// harness drives a wizard Model as Bubble Tea would: it sends a message to
// Update and runs the command Update returns, feeding its message back,
// until nothing is left to run.
type harness struct {
	t       *testing.T
	m       Model
	quit    bool
	commits [][]string // files passed to Commit
	gh      *gh.Fake
}

// testDeps returns fakes for a project in dir (a git repo whose origin is
// testRepo) with every check passing.
func testDeps(h *harness, dir string) Deps {
	return Deps{
		Home:      testHome,
		Target:    testRepo,
		Getwd:     func() (string, error) { return dir, nil },
		LookupEnv: func(string) (string, bool) { return "", false },
		TopLevel: func(_ context.Context, path string) (string, error) {
			if path == dir || strings.HasPrefix(path, dir+"/") {
				return dir, nil
			}
			return "", errors.New("not a git repository")
		},
		RemoteURL: func(context.Context, string, string) (string, error) {
			return "git@github.com:" + testRepo + ".git", nil
		},
		Commit: func(_ context.Context, _, _ string, files ...string) error {
			h.commits = append(h.commits, files)
			return nil
		},
		GH:       h.gh,
		CheckCLI: func(context.Context, string) (string, error) { return "1.0.0", nil },
		Validate: func(context.Context, agents.CLI, string, string) error { return nil },
	}
}

// newHarness starts a wizard on c in folder dir and lets its detection of
// the git repo finish. change, if set, adjusts the fake dependencies.
func newHarness(t *testing.T, dir string, c config.Config, change func(*Deps)) *harness {
	t.Helper()
	h := &harness{t: t, gh: &gh.Fake{Login: "sohaibseniorswe-sys"}}
	deps := testDeps(h, dir)
	if change != nil {
		change(&deps)
	}
	h.m = New(deps, c)
	// Init also starts the spinner, whose timer the tests do not need.
	h.run(h.m.detectFolder())
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	next, cmd := h.m.Update(msg)
	h.m = next.(Model)
	h.run(cmd)
}

// run runs cmd and sends its message, if any.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.QuitMsg:
		h.quit = true
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	default:
		h.send(msg)
	}
}

// keys sends key presses by name ("enter", "down", "esc") or, for
// anything else, as typed text.
func (h *harness) keys(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		switch k {
		case "enter":
			h.send(tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			h.send(tea.KeyMsg{Type: tea.KeyEsc})
		case "up":
			h.send(tea.KeyMsg{Type: tea.KeyUp})
		case "down":
			h.send(tea.KeyMsg{Type: tea.KeyDown})
		case "left":
			h.send(tea.KeyMsg{Type: tea.KeyLeft})
		case "right":
			h.send(tea.KeyMsg{Type: tea.KeyRight})
		case "ctrl+c":
			h.send(tea.KeyMsg{Type: tea.KeyCtrlC})
		case "backspace":
			h.send(tea.KeyMsg{Type: tea.KeyBackspace})
		default:
			h.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

func (h *harness) wantStep(want int) {
	h.t.Helper()
	if h.m.step != want {
		h.t.Fatalf("step = %d (%s), want %d (%s)\n%s", h.m.step, stepNames[h.m.step-1], want, stepNames[want-1], h.m.View())
	}
}

// palantirConfig is the settings behind screens 11–13.
func palantirConfig() config.Config {
	c := config.Default(testRepo)
	c.Agents.PM = config.Agent{CLI: agents.ClaudeID, Model: "opus", Effort: "high"}
	c.Agents.Companion = config.Companion{Agent: config.Agent{CLI: agents.ClaudeID, Model: "sonnet", Effort: "medium"}}
	return c
}

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
		t.Fatalf("%v (run go test ./internal/wizard -update to create it)", err)
	}
	if view != string(want) {
		t.Errorf("%s differs from %s (go test ./internal/wizard -update rewrites it):\n%s", name, path, view)
	}
}

const palantirDir = testHome + "/projects/palantir"

func TestViewPMAgent(t *testing.T) { // screen 11
	h := newHarness(t, palantirDir, palantirConfig(), nil)
	h.keys("enter", "enter", "enter") // folder, repo, Dev agent
	h.wantStep(stepPM)
	h.keys("down") // focus the model chips
	checkGolden(t, "11-wizard-pm", h.m.View())
}

func TestViewCompanion(t *testing.T) { // screen 12
	h := newHarness(t, palantirDir, palantirConfig(), nil)
	h.keys("enter", "enter", "enter", "enter")
	h.wantStep(stepComp)
	h.keys("down", "down", "down") // Source → CLI → Model → Effort
	checkGolden(t, "12-wizard-companion", h.m.View())
}

func TestViewAutonomy(t *testing.T) { // screen 13
	h := newHarness(t, palantirDir, palantirConfig(), nil)
	h.keys("enter", "enter", "enter", "enter", "enter") // to step 6, whose checks pass
	h.wantStep(stepChecks)
	if h.m.checking {
		t.Fatal("step 6 still checking")
	}
	h.keys("enter")
	h.wantStep(stepAutonomy)
	checkGolden(t, "13-wizard-autonomy", h.m.View())
}

func TestFolderStep(t *testing.T) {
	// Not in a git repo: the folder is asked for and checked.
	h := newHarness(t, palantirDir, palantirConfig(), func(d *Deps) {
		d.Getwd = func() (string, error) { return "/tmp", nil }
	})
	if h.m.folder.Value() != "" {
		t.Errorf("folder pre-filled with %q, want empty", h.m.folder.Value())
	}
	h.keys("/srv/nothing", "enter")
	h.wantStep(stepFolder)
	if !strings.Contains(h.m.View(), "/srv/nothing is not inside a git repo") {
		t.Errorf("no error shown:\n%s", h.m.View())
	}
	for range len("/srv/nothing") {
		h.keys("backspace")
	}
	h.keys("~/projects/palantir/cmd", "enter") // "~" and a subfolder are fine
	h.wantStep(stepRepo)
	if h.m.dir != palantirDir {
		t.Errorf("dir = %q", h.m.dir)
	}
	if !strings.Contains(h.m.View(), "sohaibseniorswe-sys/palantir   (origin)") {
		t.Errorf("repo not shown:\n%s", h.m.View())
	}
}

func TestRepoStepErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Deps, *gh.Fake)
		want   string
	}{
		{"origin is another repo", func(d *Deps, _ *gh.Fake) {
			d.RemoteURL = func(context.Context, string, string) (string, error) {
				return "https://github.com/sohaibseniorswe-sys/huddle-sandbox", nil
			}
		}, "run huddle --repo sohaibseniorswe-sys/huddle-sandbox setup"},
		{"gh not signed in", func(_ *Deps, f *gh.Fake) { f.AuthErr = errors.New("no") }, "gh auth login"},
		{"read only", func(_ *Deps, f *gh.Fake) { f.Perm = "READ" }, "Huddle needs write access"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fake *gh.Fake
			h := newHarness(t, palantirDir, palantirConfig(), func(d *Deps) {
				fake = d.GH.(*gh.Fake)
				tt.change(d, fake)
			})
			h.keys("enter")
			h.wantStep(stepRepo)
			if !strings.Contains(h.m.repoErr, tt.want) {
				t.Errorf("error %q lacks %q", h.m.repoErr, tt.want)
			}
			h.keys("enter") // tries again, still failing
			h.wantStep(stepRepo)
		})
	}
}

// claudeBadModel is Claude Code's real answer to an unknown model (2.1.299).
const claudeBadModel = `{"type":"result","subtype":"success","is_error":true,"api_error_status":404,"session_id":"84f0","result":"There's an issue with the selected model (opux). It may not exist or you may not have access to it. Run --model to pick a different model."}`

// fakeClaude writes a script that answers like Claude Code rejecting a
// model, and returns its path.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out.json")
	if err := os.WriteFile(out, []byte(claudeBadModel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat > /dev/null\ncat '" + out + "'\nexit 1\n"
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// A bad custom model name: step 6 shows the CLI's own error, from the real
// agents.Validate running a fake CLI, and Enter goes back to the role.
func TestBadCustomModelGoesBackToItsStep(t *testing.T) {
	script := fakeClaude(t)
	h := newHarness(t, palantirDir, palantirConfig(), func(d *Deps) {
		d.Validate = func(ctx context.Context, cli agents.CLI, model, effort string) error {
			if cli.ID == agents.ClaudeID && model == "opux" {
				cli.Binary = script
				return agents.Validate(ctx, cli, model, effort)
			}
			return nil
		}
	})
	h.keys("enter", "enter", "enter") // to the PM step
	h.wantStep(stepPM)
	h.keys("down", "right", "right", "right", "right") // Model → Custom…
	if !h.m.roles[rolePM].isCustom() {
		t.Fatalf("Custom… not selected:\n%s", h.m.View())
	}
	h.keys("down", "opux", "enter", "enter") // type the name, confirm PM and Companion
	h.wantStep(stepChecks)
	view := h.m.View()
	if !strings.Contains(view, `Claude Code rejected model "opux"`) || !strings.Contains(view, "Enter goes back to the PM step") {
		t.Fatalf("step 6 does not show the error:\n%s", view)
	}
	h.keys("enter")
	h.wantStep(stepPM)
	if got := h.m.roles[rolePM].modelValue(); got != "opux" {
		t.Errorf("custom model = %q, want it kept", got)
	}
}

func TestChangingCLIResetsEffort(t *testing.T) {
	c := palantirConfig()
	c.Agents.Dev = config.Agent{CLI: agents.CodexID, Effort: "none"} // a Codex-only effort
	h := newHarness(t, palantirDir, c, nil)
	h.keys("enter", "enter")
	h.wantStep(stepDev)
	if got := h.m.roles[roleDev].effortValue(); got != "none" {
		t.Fatalf("effort = %q, want none pre-selected", got)
	}
	h.keys("left") // CLI: Codex → Claude Code
	d := h.m.roles[roleDev]
	if d.agentCLI().ID != agents.ClaudeID || d.effortValue() != agents.Claude().DefaultEffort || d.modelValue() != "opus" {
		t.Errorf("after the CLI change: %s, want Claude Code's first model and default effort", d.summary())
	}
	if !agents.Claude().ValidEffort(d.effortValue()) {
		t.Errorf("effort %q is not valid for Claude Code", d.effortValue())
	}
}

func TestConflicts(t *testing.T) {
	sample := "# Rules\n- Never push to main directly.\n- Run the tests.\n- Ask before deleting files.\n- Wait for approval on big changes. Do not merge on Fridays.\n"
	got := Conflicts(sample, 8)
	if len(got) != 3 || got[0].Line != 2 || got[1].Line != 4 || got[2].Line != 5 {
		t.Errorf("Conflicts at 8 = %+v, want lines 2, 4, 5", got)
	}
	if got := Conflicts(sample, 3); len(got) != 0 {
		t.Errorf("Conflicts at 3 = %+v, want none", got)
	}
	if got := Conflicts(agentsTemplate, 10); len(got) != 0 {
		t.Errorf("the starter AGENTS.md fights autonomy 10: %+v", got)
	}
}

// gitDir returns a temp folder standing in for a clone, with files.
func gitDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// toPrepare walks h from step 1 to step 8 at autonomy level.
func toPrepare(h *harness, level int) {
	h.t.Helper()
	h.keys("enter", "enter", "enter", "enter", "enter", "enter") // to step 7
	h.wantStep(stepAutonomy)
	for h.m.autonomy > level {
		h.keys("left")
	}
	for h.m.autonomy < level {
		h.keys("right")
	}
	h.keys("enter")
	h.wantStep(stepPrepare)
}

func TestPrepareWarnsAboutConflictingRules(t *testing.T) {
	agentsMD := "# Rules\n- Branch per issue.\n- Never push without review.\n- Wait for approval before merging.\n"
	for _, tt := range []struct {
		level int
		warn  bool
	}{{8, true}, {3, false}} {
		dir := gitDir(t, map[string]string{"AGENTS.md": agentsMD, "CLAUDE.md": "Read AGENTS.md.\n"})
		h := newHarness(t, dir, palantirConfig(), nil)
		toPrepare(h, tt.level)
		view := h.m.View()
		warned := strings.Contains(view, "fights autonomy") && strings.Contains(view, "AGENTS.md:3  - Never push without review.") &&
			strings.Contains(view, "AGENTS.md:4  - Wait for approval before merging.")
		if warned != tt.warn {
			t.Errorf("autonomy %d: warned = %v, want %v\n%s", tt.level, warned, tt.warn, view)
		}
		h.keys("enter") // never blocks
		h.wantStep(stepSave)
	}
}

func TestSaveWritesSettingsAndStarterFiles(t *testing.T) {
	dir := gitDir(t, nil) // no AGENTS.md, no CLAUDE.md
	home := t.TempDir()
	h := newHarness(t, dir, config.Default(testRepo), func(d *Deps) { d.Home = home })
	toPrepare(h, 8)
	if got := h.gh.Created; len(got) != 7 {
		t.Errorf("labels created = %v, want all 7", got)
	}
	view := h.m.View()
	if !strings.Contains(view, "AGENTS.md missing") || !strings.Contains(view, "Starter AGENTS.md:") || !strings.Contains(view, "│ # Agent rules") {
		t.Fatalf("starter not shown before writing:\n%s", view)
	}
	h.keys("down", "right", "enter") // keep AGENTS.md, skip CLAUDE.md
	h.wantStep(stepSave)
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
		t.Fatal("AGENTS.md written before step 9")
	}
	h.keys("enter")
	if !h.quit {
		t.Fatalf("wizard did not finish:\n%s", h.m.View())
	}
	saved, ok := h.m.Saved()
	if !ok || saved.Dir != dir || saved.Autonomy != 8 || saved.Agents.Dev.Effort != "medium" {
		t.Errorf("Saved() = %+v, %v", saved, ok)
	}
	loaded, exists, err := config.Load(home, testRepo)
	if err != nil || !exists || loaded.Dir != dir || loaded.Validate() != nil {
		t.Errorf("settings file: %+v, exists %v, err %v", loaded, exists, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md")); err != nil || string(data) != agentsTemplate {
		t.Errorf("AGENTS.md = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("CLAUDE.md written though skipped")
	}
	if len(h.commits) != 1 || strings.Join(h.commits[0], ",") != "AGENTS.md" {
		t.Errorf("commits = %v, want one of AGENTS.md", h.commits)
	}
}

// huddle setup: every current value is pre-selected.
func TestSetupPreselectsCurrentValues(t *testing.T) {
	c := config.Config{
		Repo: testRepo,
		Dir:  "/elsewhere",
		Agents: config.Agents{
			Dev:       config.Agent{CLI: agents.ClaudeID, Model: "claude-opus-5-5", Effort: "xhigh"},
			PM:        config.Agent{CLI: agents.CodexID, Model: "gpt-6.1-sol", Effort: "low"},
			Companion: config.Companion{Agent: config.Agent{CLI: agents.ClaudeID, Model: "haiku", Effort: "low"}},
		},
		Autonomy: 4,
	}
	h := newHarness(t, palantirDir, c, nil)
	if got := h.m.folder.Value(); got != palantirDir {
		t.Errorf("folder = %q, want the detected repo", got)
	}
	h.keys("enter", "enter")
	got := h.m.config()
	got.Dir = c.Dir
	if got.Agents != c.Agents || got.Autonomy != 4 {
		t.Errorf("pre-selected %+v, want %+v", got, c)
	}
	if !h.m.roles[roleDev].isCustom() {
		t.Error("an unknown model is not shown as Custom…")
	}
	// No folder in the settings and none detected: the input starts empty;
	// a folder in the settings is used when none is detected.
	h = newHarness(t, palantirDir, c, func(d *Deps) { d.Getwd = func() (string, error) { return "/tmp", nil } })
	if got := h.m.folder.Value(); got != "/elsewhere" {
		t.Errorf("folder = %q, want the one in the settings", got)
	}
}

func TestEscGoesBackOneStepKeepingChoices(t *testing.T) {
	dir := gitDir(t, map[string]string{"AGENTS.md": "x\n", "CLAUDE.md": "y\n"})
	h := newHarness(t, dir, palantirConfig(), nil)
	h.keys("enter", "enter")
	h.keys("enter")                          // Dev as it is
	h.keys("down", "down", "right", "enter") // PM: effort xhigh
	h.keys("left", "enter")                  // Companion: same as PM
	h.keys("enter")                          // checks pass
	h.keys("left", "left", "enter")          // autonomy 6
	h.keys("enter")                          // prepare
	h.wantStep(stepSave)
	want := h.m.config()
	for s := stepSave; s > stepFolder; s-- {
		h.wantStep(s)
		h.keys("esc")
		if got := h.m.config(); got != want {
			t.Fatalf("Esc at step %d changed the choices: %+v, want %+v", s, got, want)
		}
	}
	h.wantStep(stepFolder)
	h.keys("esc") // nothing before step 1
	h.wantStep(stepFolder)
	if h.quit {
		t.Error("Esc at step 1 quit")
	}
}

func TestCtrlCQuitsWithoutSaving(t *testing.T) {
	home := t.TempDir()
	h := newHarness(t, palantirDir, palantirConfig(), func(d *Deps) { d.Home = home })
	h.keys("enter", "enter", "ctrl+c")
	if !h.quit {
		t.Fatal("ctrl+c did not quit")
	}
	if _, ok := h.m.Saved(); ok {
		t.Error("Saved() after ctrl+c")
	}
	if _, exists, _ := config.Load(home, testRepo); exists {
		t.Error("a settings file was written")
	}
}

func TestClickAChip(t *testing.T) {
	h := newHarness(t, palantirDir, palantirConfig(), nil)
	h.keys("enter", "enter", "enter")
	h.wantStep(stepPM)
	// Find "Haiku 5.5" on screen and click it.
	lines := strings.Split(h.m.View(), "\n")
	for y, line := range lines {
		if x := strings.Index(line, "Haiku 5.5"); x >= 0 {
			x = lipgloss.Width(line[:x])
			h.send(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft})
			break
		}
	}
	if got := h.m.roles[rolePM].modelValue(); got != "haiku" {
		t.Errorf("model after the click = %q, want haiku", got)
	}
	if f, _ := h.m.focusedField(); f.id != fieldModel {
		t.Errorf("focused field = %v, want the model", f.id)
	}
}
