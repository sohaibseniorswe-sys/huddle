package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCommands(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{"dev fresh", Dev("/repo", "do it", "", "/logs/last.txt"), `codex exec --strict-config --ignore-user-config ` +
			`-c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message /logs/last.txt -`},
		{"dev resume", Dev("/repo", "do it", "01a132be-f0bc-7293-89cc-72f23dcb576c", "/logs/last.txt"), `codex exec resume --strict-config --ignore-user-config ` +
			`-c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message /logs/last.txt ` +
			`01a132be-f0bc-7293-89cc-72f23dcb576c -`},
		{"pm fresh", PM("/repo", "decide", ""), `claude -p --output-format json --dangerously-skip-permissions`},
		{"pm resume", PM("/repo", "decide", "a1c710e8-9bf5-4507-af87-2d493feb051c"),
			`claude -p --output-format json --dangerously-skip-permissions --resume a1c710e8-9bf5-4507-af87-2d493feb051c`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(append([]string{tt.spec.Name}, tt.spec.Args...), " ")
			if got != tt.want {
				t.Errorf("command = %s\nwant      %s", got, tt.want)
			}
			if tt.spec.Dir != "/repo" {
				t.Errorf("Dir = %q, want /repo", tt.spec.Dir)
			}
			if tt.spec.Stdin == "" {
				t.Error("prompt must go on stdin")
			}
			for _, a := range tt.spec.Args {
				if a == tt.spec.Stdin {
					t.Error("prompt must not be a command-line argument")
				}
				for _, banned := range []string{"--add-dir", "--permission-mode", "--last", "--continue"} {
					if a == banned {
						t.Errorf("command contains %s", banned)
					}
				}
			}
		})
	}
}

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
		res  Result
		want string
	}{
		{"recorded fresh", readTestdata(t, "codex-fresh.last"), Result{Stdout: "OK\n"}, "OK"},
		{"recorded resume", readTestdata(t, "codex-resume.last"), Result{Stdout: "AGAIN\n"}, "AGAIN"},
		{"no file, stdout", "", Result{Stdout: "Done.\n", Tail: "tokens used\n1,234"}, "Done."},
		{"no file, no stdout", "", Result{Tail: "Error: thread/resume failed"}, "Error: thread/resume failed"},
		{"empty file", "\n", Result{Stdout: "from stdout"}, "from stdout"},
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
			if got := ClaudeFinalMessage(Result{Stdout: stdout}); got != tt.result {
				t.Errorf("ClaudeFinalMessage() = %q, want %q", got, tt.result)
			}
		})
	}
	for _, bad := range []string{"", "No conversation found with session ID: 1111\n", "{not json"} {
		if _, err := ParseClaude(bad); err == nil {
			t.Errorf("ParseClaude(%q) error = nil, want an error", bad)
		}
		if got := ClaudeFinalMessage(Result{Stdout: bad, Tail: "the tail"}); got != "the tail" {
			t.Errorf("ClaudeFinalMessage(%q) = %q, want the tail", bad, got)
		}
	}
}
