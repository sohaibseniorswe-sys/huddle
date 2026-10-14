package agents

import (
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// The fixed parts of each command, to keep the table below readable.
const (
	codexFull   = `-c sandbox_mode="danger-full-access" -c approval_policy="never"`
	codexRO     = `-c sandbox_mode="read-only" -c approval_policy="never"`
	claudeRO    = "-p --output-format json --tools Read,Grep,Glob,WebSearch,WebFetch,Bash --permission-mode dontAsk --strict-mcp-config --setting-sources user"
	allowedList = "--allowedTools Read Grep Glob WebSearch WebFetch Bash(git log:*) Bash(git show:*) Bash(git diff:*) " +
		"Bash(git status:*) Bash(git blame:*) Bash(git ls-files:*) Bash(git grep:*) Bash(git rev-parse:*) " +
		"Bash(gh issue view:*) Bash(gh issue list:*) Bash(gh pr view:*) Bash(gh pr list:*) Bash(gh pr checks:*) " +
		"Bash(gh pr diff:*) Bash(gh run list:*) Bash(gh run view:*)"
)

func TestCommand(t *testing.T) {
	codex := Choice{CLI: Codex()}
	codexSet := Choice{CLI: Codex(), Model: "gpt-6.1-sol", Effort: "high"}
	claude := Choice{CLI: Claude()}
	claudeSet := Choice{CLI: Claude(), Model: "sonnet", Effort: "xhigh"}
	const id = "01a132be-f0bc-7293-89cc-72f23dcb576c"
	const last = "/logs/run.last"

	tests := []struct {
		name      string
		role      session.Role
		choice    Choice
		sessionID string
		want      string
	}{
		// Every role on Codex, without and with a model and effort.
		{"dev codex", session.Dev, codex, "",
			"codex exec --strict-config --ignore-user-config " + codexFull + " --output-last-message /logs/run.last -"},
		{"dev codex set", session.Dev, codexSet, "",
			"codex exec --strict-config --ignore-user-config " + codexFull + ` -m gpt-6.1-sol -c model_reasoning_effort="high" --output-last-message /logs/run.last -`},
		{"pm codex", session.PM, codex, "",
			"codex exec --strict-config --ignore-user-config " + codexFull + " --output-last-message /logs/run.last -"},
		{"pm codex set", session.PM, codexSet, "",
			"codex exec --strict-config --ignore-user-config " + codexFull + ` -m gpt-6.1-sol -c model_reasoning_effort="high" --output-last-message /logs/run.last -`},
		{"companion codex", session.Companion, codex, "",
			"codex exec --strict-config --ignore-user-config " + codexRO + " --output-last-message /logs/run.last -"},
		{"companion codex set", session.Companion, codexSet, "",
			"codex exec --strict-config --ignore-user-config " + codexRO + ` -m gpt-6.1-sol -c model_reasoning_effort="high" --output-last-message /logs/run.last -`},

		// Every role on Claude Code, without and with a model and effort.
		{"dev claude", session.Dev, claude, "",
			"claude -p --output-format json --dangerously-skip-permissions"},
		{"dev claude set", session.Dev, claudeSet, "",
			"claude -p --output-format json --dangerously-skip-permissions --model sonnet --effort xhigh"},
		{"pm claude", session.PM, claude, "",
			"claude -p --output-format json --dangerously-skip-permissions"},
		{"pm claude set", session.PM, claudeSet, "",
			"claude -p --output-format json --dangerously-skip-permissions --model sonnet --effort xhigh"},
		{"companion claude", session.Companion, claude, "",
			"claude " + claudeRO + " " + allowedList},
		{"companion claude set", session.Companion, claudeSet, "",
			"claude " + claudeRO + " --model sonnet --effort xhigh " + allowedList},

		// Only a model, or only an effort.
		{"dev codex model only", session.Dev, Choice{CLI: Codex(), Model: "gpt-6.1-sol"}, "",
			"codex exec --strict-config --ignore-user-config " + codexFull + " -m gpt-6.1-sol --output-last-message /logs/run.last -"},
		{"pm claude effort only", session.PM, Choice{CLI: Claude(), Effort: "max"}, "",
			"claude -p --output-format json --dangerously-skip-permissions --effort max"},

		// Resumed sessions: Codex puts the id last, before "-"; Claude Code
		// before --allowedTools, which must stay last.
		{"dev codex resume", session.Dev, codexSet, id,
			"codex exec resume --strict-config --ignore-user-config " + codexFull + ` -m gpt-6.1-sol -c model_reasoning_effort="high" --output-last-message /logs/run.last ` + id + " -"},
		{"pm codex resume", session.PM, codex, id,
			"codex exec resume --strict-config --ignore-user-config " + codexFull + " --output-last-message /logs/run.last " + id + " -"},
		{"companion codex resume", session.Companion, codex, id,
			"codex exec resume --strict-config --ignore-user-config " + codexRO + " --output-last-message /logs/run.last " + id + " -"},
		{"dev claude resume", session.Dev, claude, id,
			"claude -p --output-format json --dangerously-skip-permissions --resume " + id},
		{"pm claude resume", session.PM, claudeSet, id,
			"claude -p --output-format json --dangerously-skip-permissions --model sonnet --effort xhigh --resume " + id},
		{"companion claude resume", session.Companion, claudeSet, id,
			"claude " + claudeRO + " --model sonnet --effort xhigh --resume " + id + " " + allowedList},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := Command(tt.role, tt.choice, tt.sessionID, last)
			got := strings.Join(append([]string{spec.Name}, spec.Args...), " ")
			if got != tt.want {
				t.Errorf("command = %s\nwant      %s", got, tt.want)
			}
			for _, a := range spec.Args {
				for _, banned := range []string{"--add-dir", "--last", "--continue"} {
					if a == banned {
						t.Errorf("command contains %s", banned)
					}
				}
			}
		})
	}
}

// TestCommandUsesTheBinary checks that the command runs the CLI's Binary,
// which the Validate tests point at fake CLIs.
func TestCommandUsesTheBinary(t *testing.T) {
	cli := Claude()
	cli.Binary = "/tmp/fake-claude"
	if got := Command(session.PM, Choice{CLI: cli}, "", "").Name; got != "/tmp/fake-claude" {
		t.Errorf("Name = %q, want the CLI's Binary", got)
	}
}

// TestCompanionCommandIsReadOnly checks that no companion command, on
// either CLI, names a tool or mode that writes.
func TestCompanionCommandIsReadOnly(t *testing.T) {
	for _, cli := range All() {
		for _, id := range []string{"", "c02b615b"} {
			spec := Command(session.Companion, Choice{CLI: cli, Model: "m", Effort: "high"}, id, "/l")
			for _, a := range spec.Args {
				for _, banned := range []string{"Edit", "Write", "NotebookEdit", "--dangerously-skip-permissions",
					"bypassPermissions", "danger-full-access", "workspace-write"} {
					if strings.Contains(a, banned) {
						t.Errorf("%s companion argument %q contains %s", cli.Name, a, banned)
					}
				}
			}
		}
	}
}
