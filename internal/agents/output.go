package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
)

// This file reads what a finished run left behind: its session id, its
// final message and the model it used. Read does it for either CLI; the
// functions below it do it for one.

// Output is what Huddle reads from one finished run, whatever the CLI.
type Output struct {
	SessionID string // the run's session id; "" if none was found
	Message   string // the final message (or the output's tail)
	// Failed means the CLI reported an error: a non-zero exit, or Claude
	// Code's envelope saying is_error.
	Failed bool
	// Model is the model the run used, as the CLI reported it ("" if it
	// did not): Codex's header line "model: ...", or the model in Claude
	// Code's envelope. It shows what "CLI default" resolved to.
	Model string
}

// Read reads a finished run of cli. lastMessage is the file Codex wrote
// its final message to (see Command).
func Read(cli CLI, res runner.Result, lastMessage string) Output {
	out := Output{Failed: res.ExitCode != 0}
	if cli.ID == ClaudeID {
		out.Message = ClaudeFinalMessage(res)
		if env, err := ParseClaude(res.Stdout); err == nil {
			out.SessionID = env.SessionID
			out.Failed = out.Failed || env.IsError
			out.Model = env.Model()
		}
		return out
	}
	out.SessionID = CodexSessionID(res.Head)
	out.Message = CodexFinalMessage(lastMessage, res)
	out.Model = CodexModel(res.Head)
	return out
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

// codexModelRe matches the "model: <name>" line of codex exec's header.
var codexModelRe = regexp.MustCompile(`(?m)^model: (\S+)\s*$`)

// CodexModel returns the model named in codex exec's header (Result.Head
// is enough), or "" if there is none.
func CodexModel(output string) string {
	if m := codexModelRe.FindStringSubmatch(output); m != nil {
		return m[1]
	}
	return ""
}

// CodexFinalMessage returns codex's final message: the --output-last-message
// file if it is non-empty, otherwise its stdout (where codex exec prints only
// the final message), otherwise the tail of its output.
func CodexFinalMessage(lastMessagePath string, res runner.Result) string {
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
	// ModelUsage has one entry per model the run used, keyed by the
	// model's full name ("claude-opus-5-5").
	ModelUsage map[string]ClaudeModelUsage `json:"modelUsage"`
}

// ClaudeModelUsage is the part of one modelUsage entry that Huddle uses.
type ClaudeModelUsage struct {
	OutputTokens int `json:"outputTokens"`
}

// Model returns the model the run used, or "" if the envelope names none.
// A run that started a subagent on another model names several; the one
// that wrote the most is the run's own model.
func (e ClaudeEnvelope) Model() string {
	best, most := "", -1
	for name, u := range e.ModelUsage {
		// Ties are broken by name, so the answer never depends on the
		// map's random order.
		if u.OutputTokens > most || u.OutputTokens == most && name < best {
			best, most = name, u.OutputTokens
		}
	}
	return best
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
func ClaudeFinalMessage(res runner.Result) string {
	if env, err := ParseClaude(res.Stdout); err == nil && strings.TrimSpace(env.Result) != "" {
		return strings.TrimSpace(env.Result)
	}
	return res.Tail
}
