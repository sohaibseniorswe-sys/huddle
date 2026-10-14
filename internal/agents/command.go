package agents

import (
	"fmt"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// This file builds the command Huddle runs for one role. Every command
// reads its prompt from stdin, which avoids argument length limits; the
// caller puts the prompt in Spec.Stdin and the repo root in Spec.Dir.
//
// The Dev and the PM run with full permissions and no allow or deny lists
// (MVP_PLAN.md section 6), whichever CLI plays them. The companion is
// read-only on purpose: it talks with the human while the others work, and
// only agreed decisions may change the project
// (docs/v3/issues/02-companion-and-decisions.md).

// codexFullAccess are the codex config overrides for a Dev or PM turn. They
// are passed with -c because `codex exec resume` has no --sandbox flag.
var codexFullAccess = []string{
	`sandbox_mode="danger-full-access"`,
	`approval_policy="never"`,
}

// codexReadOnly are the codex config overrides for the companion: its shell
// cannot write files. The read-only sandbox also has no network, so a Codex
// companion cannot run gh and relies on Huddle's context block.
var codexReadOnly = []string{
	`sandbox_mode="read-only"`,
	`approval_policy="never"`,
}

// CompanionTools are the only built-in Claude Code tools the companion
// gets (--tools): it can read files, search them, search and fetch the web,
// and run shell commands, but it has no tool that edits or writes files.
var CompanionTools = []string{"Read", "Grep", "Glob", "WebSearch", "WebFetch", "Bash"}

// CompanionAllowed are the permission rules a Claude Code companion runs
// with (--allowedTools). With --permission-mode dontAsk every tool call
// that no rule allows is denied, so the shell can run only these read-only
// git and gh commands. Commands joined with && or ; are checked one by one.
var CompanionAllowed = []string{
	"Read", "Grep", "Glob", "WebSearch", "WebFetch",
	"Bash(git log:*)", "Bash(git show:*)", "Bash(git diff:*)", "Bash(git status:*)",
	"Bash(git blame:*)", "Bash(git ls-files:*)", "Bash(git grep:*)", "Bash(git rev-parse:*)",
	"Bash(gh issue view:*)", "Bash(gh issue list:*)",
	"Bash(gh pr view:*)", "Bash(gh pr list:*)", "Bash(gh pr checks:*)", "Bash(gh pr diff:*)",
	"Bash(gh run list:*)", "Bash(gh run view:*)",
}

// Command returns the command for one run of role played by c. An empty
// sessionID starts a fresh session; otherwise that session is resumed by
// id (never "the last session", which could pick up another session in the
// same folder). lastMessage is the file Codex writes its final message to;
// Claude Code ignores it, because its final message is in its JSON
// envelope.
func Command(role session.Role, c Choice, sessionID, lastMessage string) runner.Spec {
	var args []string
	if c.CLI.ID == ClaudeID {
		args = claudeArgs(role, c, sessionID)
	} else {
		args = codexArgs(role, c, sessionID, lastMessage)
	}
	return runner.Spec{Name: c.CLI.Binary, Args: args}
}

// codexArgs are the arguments of `codex exec`:
//
//	codex exec [resume] --strict-config --ignore-user-config -c <access>...
//	  [-m <model>] [-c model_reasoning_effort="<effort>"]
//	  --output-last-message <file> [<session-id>] -
//
// --ignore-user-config skips ~/.codex/config.toml, so a user's config
// cannot change the run (the login still comes from CODEX_HOME), and
// --strict-config rejects a misspelt -c key instead of ignoring it.
// `codex exec resume` takes the same options, before the session id. The
// final "-" reads the prompt from stdin.
func codexArgs(role session.Role, c Choice, sessionID, lastMessage string) []string {
	args := []string{"exec"}
	if sessionID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--strict-config", "--ignore-user-config")
	access := codexFullAccess
	if role == session.Companion {
		access = codexReadOnly
	}
	for _, kv := range access {
		args = append(args, "-c", kv)
	}
	args = append(args, codexModelArgs(c)...)
	args = append(args, "--output-last-message", lastMessage)
	if sessionID != "" {
		args = append(args, sessionID)
	}
	return append(args, "-")
}

// codexModelArgs are the model and effort flags for codex; an empty value
// leaves its flag out.
func codexModelArgs(c Choice) []string {
	var args []string
	if c.Model != "" {
		args = append(args, "-m", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%q", c.Effort))
	}
	return args
}

// claudeArgs are the arguments of `claude -p` (print mode, one prompt and
// one answer). Its stdout is a JSON envelope; see ParseClaude.
//
// The Dev and PM get --dangerously-skip-permissions. The companion gets
// only reading tools, denies every tool call its allow list does not name,
// loads no MCP servers (--strict-mcp-config without --mcp-config), and
// loads only the user's own settings file (--setting-sources user), so
// allow rules in a target repo's .claude/settings.json or
// .claude/settings.local.json can never give it write access.
func claudeArgs(role session.Role, c Choice, sessionID string) []string {
	args := []string{"-p", "--output-format", "json"}
	if role == session.Companion {
		args = append(args,
			"--tools", strings.Join(CompanionTools, ","),
			"--permission-mode", "dontAsk",
			"--strict-mcp-config",
			"--setting-sources", "user")
	} else {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, claudeModelArgs(c)...)
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	if role == session.Companion {
		// --allowedTools takes every argument after it, so it goes last.
		args = append(args, "--allowedTools")
		args = append(args, CompanionAllowed...)
	}
	return args
}

// claudeModelArgs are the model and effort flags for Claude Code; an empty
// value leaves its flag out.
func claudeModelArgs(c Choice) []string {
	var args []string
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if c.Effort != "" {
		args = append(args, "--effort", c.Effort)
	}
	return args
}
