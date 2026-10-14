package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
)

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The testdata files are recorded real output (codex-cli 0.160.0, Claude Code
// 2.1.296): a fresh run asked to remember a word, then a resume of the same
// session. codex-*.out is stderr followed by stdout; codex-*.last is the
// --output-last-message file.

func TestCodexSessionID(t *testing.T) {
	for _, name := range []string{"codex-fresh.out", "codex-resume.out"} {
		t.Run(name, func(t *testing.T) {
			if got, want := CodexSessionID(readTestdata(t, name)), "01a132be-f0bc-7293-89cc-72f23dcb576c"; got != want {
				t.Errorf("CodexSessionID() = %q, want %q", got, want)
			}
		})
	}
	bad := "Error: thread/resume: thread/resume failed: no rollout found for thread id 1111 (code -32600)\n"
	if got := CodexSessionID(bad); got != "" {
		t.Errorf("CodexSessionID(failed resume) = %q, want empty", got)
	}
}

func TestCodexFinalMessage(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		last string // "" means no file
		res  runner.Result
		want string
	}{
		{"recorded fresh", readTestdata(t, "codex-fresh.last"), runner.Result{Stdout: "OK\n"}, "OK"},
		{"recorded resume", readTestdata(t, "codex-resume.last"), runner.Result{Stdout: "AGAIN\n"}, "AGAIN"},
		{"no file, stdout", "", runner.Result{Stdout: "Done.\n", Tail: "tokens used\n1,234"}, "Done."},
		{"no file, no stdout", "", runner.Result{Tail: "Error: thread/resume failed"}, "Error: thread/resume failed"},
		{"empty file", "\n", runner.Result{Stdout: "from stdout"}, "from stdout"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, strings.Repeat("x", i+1))
			if tt.last != "" {
				if err := os.WriteFile(path, []byte(tt.last), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := CodexFinalMessage(path, tt.res); got != tt.want {
				t.Errorf("CodexFinalMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseClaude(t *testing.T) {
	tests := []struct {
		file, result string
	}{
		{"claude-fresh.json", "OK"},
		{"claude-resume.json", "PELICAN"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			stdout := readTestdata(t, tt.file)
			env, err := ParseClaude(stdout)
			if err != nil {
				t.Fatal(err)
			}
			if env.SessionID != "a1c710e8-9bf5-4507-af87-2d493feb051c" || env.Result != tt.result || env.IsError || env.Type != "result" {
				t.Errorf("ParseClaude() = %+v", env)
			}
			if got := ClaudeFinalMessage(runner.Result{Stdout: stdout}); got != tt.result {
				t.Errorf("ClaudeFinalMessage() = %q, want %q", got, tt.result)
			}
		})
	}
	for _, bad := range []string{"", "No conversation found with session ID: 1111\n", "{not json"} {
		if _, err := ParseClaude(bad); err == nil {
			t.Errorf("ParseClaude(%q) error = nil, want an error", bad)
		}
		if got := ClaudeFinalMessage(runner.Result{Stdout: bad, Tail: "the tail"}); got != "the tail" {
			t.Errorf("ClaudeFinalMessage(%q) = %q, want the tail", bad, got)
		}
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	last := filepath.Join(dir, "run.last")
	if err := os.WriteFile(last, []byte("Handed over.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	codexHead := "model: gpt-6.1-sol\nsession id: 01a1-f981\n"
	claudeOut := `{"type":"result","is_error":false,"session_id":"717c","result":"Done.",` +
		`"modelUsage":{"claude-haiku-5-5":{"outputTokens":12},"claude-opus-5-5":{"outputTokens":900}}}` + "\n"
	tests := []struct {
		name string
		cli  CLI
		res  runner.Result
		want Output
	}{
		{"codex", Codex(), runner.Result{Head: codexHead}, Output{SessionID: "01a1-f981", Message: "Handed over.", Model: "gpt-6.1-sol"}},
		{"codex failed", Codex(), runner.Result{ExitCode: 1, Head: codexHead}, Output{SessionID: "01a1-f981", Message: "Handed over.", Model: "gpt-6.1-sol", Failed: true}},
		{"claude", Claude(), runner.Result{Stdout: claudeOut}, Output{SessionID: "717c", Message: "Done.", Model: "claude-opus-5-5"}},
		{"claude error", Claude(), runner.Result{ExitCode: 1, Stdout: `{"is_error":true,"session_id":"9","result":"Not logged in"}`},
			Output{SessionID: "9", Message: "Not logged in", Failed: true}},
		{"claude no envelope", Claude(), runner.Result{ExitCode: 1, Tail: "No conversation found"},
			Output{Message: "No conversation found", Failed: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Read(tt.cli, tt.res, last); got != tt.want {
				t.Errorf("Read() = %+v\nwant     %+v", got, tt.want)
			}
		})
	}
}

func TestCodexModelFromRecordedHeader(t *testing.T) {
	if got := CodexModel(readTestdata(t, "codex-fresh.out")); got == "" {
		t.Error("CodexModel(recorded header) is empty")
	}
	if got := CodexModel("no header here\n"); got != "" {
		t.Errorf("CodexModel(no header) = %q, want empty", got)
	}
}

func TestByIDAndEfforts(t *testing.T) {
	for _, cli := range All() {
		got, err := ByID(cli.ID)
		if err != nil || got.Name != cli.Name {
			t.Errorf("ByID(%q) = %v, %v", cli.ID, got.Name, err)
		}
		if !cli.ValidEffort("") || !cli.ValidEffort(cli.DefaultEffort) || cli.ValidEffort("extreme") {
			t.Errorf("%s: ValidEffort is wrong", cli.Name)
		}
	}
	if _, err := ByID("gemini"); err == nil {
		t.Error("ByID(gemini) error = nil")
	}
	if Codex().ValidEffort("none") != true || Claude().ValidEffort("none") != false {
		t.Error(`"none" is a Codex effort only`)
	}
}
