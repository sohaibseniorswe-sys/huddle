package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// The agent commands are hardcoded (section 6 of the plan). Flags were checked
// against codex-cli 0.160.0 and Claude Code 2.1.296; see README.md.
//
// Both agents run with full permissions and no allow or deny lists. Both
// prompts go on stdin, which avoids argument length limits.

// DevConfig are the codex config overrides for a Dev turn. They are passed
// with -c so the command does not depend on ~/.codex/config.toml, which
// --ignore-user-config skips entirely. --strict-config makes codex reject a
// misspelt key or value instead of silently ignoring it.
var DevConfig = []string{
	`sandbox_mode="danger-full-access"`,
	`approval_policy="never"`,
}

// Dev returns the Spec for a Codex turn in dir. An empty sessionID starts a
// fresh session; otherwise that session is resumed by id (never --last, which
// could pick up another codex session in the same directory). codex writes its
// final message to lastMessagePath. "-" reads the prompt from stdin.
func Dev(dir, prompt, sessionID, lastMessagePath string) Spec {
	args := []string{"exec"}
	if sessionID != "" {
		args = append(args, "resume")
	}
	args = append(args, "--strict-config", "--ignore-user-config")
	for _, kv := range DevConfig {
		args = append(args, "-c", kv)
	}
	args = append(args, "--output-last-message", lastMessagePath)
	if sessionID != "" {
		args = append(args, sessionID)
	}
	return Spec{
		Name:  "codex",
		Args:  append(args, "-"),
		Stdin: prompt,
		Dir:   dir,
	}
}

// PM returns the Spec for a Claude Code turn in dir. An empty sessionID starts
// a fresh session; otherwise that session is resumed by id. Its stdout is a
// JSON envelope; see ParseClaude.
func PM(dir, prompt, sessionID string) Spec {
	args := []string{"-p", "--output-format", "json", "--dangerously-skip-permissions"}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	return Spec{
		Name:  "claude",
		Args:  args,
		Stdin: prompt,
		Dir:   dir,
	}
}

// CompanionTools are the only built-in Claude Code tools the companion
// gets (--tools): it can read files, search them, search and fetch the web,
// and run shell commands, but it has no tool that edits or writes files.
var CompanionTools = []string{"Read", "Grep", "Glob", "WebSearch", "WebFetch", "Bash"}

// CompanionAllowed are the permission rules the companion runs with
// (--allowedTools). With --permission-mode dontAsk every tool call that no
// rule allows is denied, so the shell can run only these read-only git and
// gh commands. Commands joined with && or ; are checked one by one.
var CompanionAllowed = []string{
	"Read", "Grep", "Glob", "WebSearch", "WebFetch",
	"Bash(git log:*)", "Bash(git show:*)", "Bash(git diff:*)", "Bash(git status:*)",
	"Bash(git blame:*)", "Bash(git ls-files:*)", "Bash(git grep:*)", "Bash(git rev-parse:*)",
	"Bash(gh issue view:*)", "Bash(gh issue list:*)",
	"Bash(gh pr view:*)", "Bash(gh pr list:*)", "Bash(gh pr checks:*)", "Bash(gh pr diff:*)",
	"Bash(gh run list:*)", "Bash(gh run view:*)",
}

// Companion returns the Spec for one companion message in dir: a Claude
// Code print-mode run that can read and research but not change anything.
// An empty sessionID starts a fresh session; otherwise that session is
// resumed by id. Like the PM's, its stdout is a JSON envelope.
//
// Unlike the Dev and PM, the companion is restricted on purpose: it talks
// with the human while the others work, and only agreed decisions may
// change the project (docs/v3/issues/02-companion-and-decisions.md).
// --strict-mcp-config without --mcp-config loads no MCP servers, whose
// tools could write.
func Companion(dir, prompt, sessionID string) Spec {
	args := []string{"-p", "--output-format", "json",
		"--tools", strings.Join(CompanionTools, ","),
		"--permission-mode", "dontAsk",
		"--strict-mcp-config"}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	// --allowedTools takes every argument after it, so it goes last.
	args = append(args, "--allowedTools")
	args = append(args, CompanionAllowed...)
	return Spec{
		Name:  "claude",
		Args:  args,
		Stdin: prompt,
		Dir:   dir,
	}
}

// codexSessionRe matches the "session id: <uuid>" line of the header codex
// exec prints (on stderr) before it starts work, for fresh and resumed runs.
var codexSessionRe = regexp.MustCompile(`(?m)^session id: ([0-9A-Za-z-]+)\s*$`)

// CodexSessionID returns the session id from codex exec output (Result.Head
// is enough), or "" if there is none, for example when codex failed early.
func CodexSessionID(output string) string {
	if m := codexSessionRe.FindStringSubmatch(output); m != nil {
		return m[1]
	}
	return ""
}

// CodexFinalMessage returns codex's final message: the --output-last-message
// file if it is non-empty, otherwise its stdout (where codex exec prints only
// the final message), otherwise the tail of its output.
func CodexFinalMessage(lastMessagePath string, res Result) string {
	if data, err := os.ReadFile(lastMessagePath); err == nil {
		if s := strings.TrimSpace(string(data)); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(res.Stdout); s != "" {
		return s
	}
	return res.Tail
}

// ClaudeEnvelope is the part of Claude Code's --output-format json envelope
// that Huddle uses.
type ClaudeEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	IsError   bool   `json:"is_error"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
}

// ParseClaude parses Claude Code's JSON envelope from its stdout. The
// envelope is the last non-empty line, so any stray output before it is
// ignored.
func ParseClaude(stdout string) (ClaudeEnvelope, error) {
	var env ClaudeEnvelope
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return env, errors.New("claude printed no JSON envelope")
	}
	if err := json.Unmarshal([]byte(last), &env); err != nil {
		return env, fmt.Errorf("parse claude JSON envelope: %w", err)
	}
	return env, nil
}

// ClaudeFinalMessage returns the envelope's result field, or the tail of the
// output if there is no usable envelope.
func ClaudeFinalMessage(res Result) string {
	if env, err := ParseClaude(res.Stdout); err == nil && strings.TrimSpace(env.Result) != "" {
		return strings.TrimSpace(env.Result)
	}
	return res.Tail
}
