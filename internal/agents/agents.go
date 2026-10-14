// Package agents describes the agent CLIs Huddle can drive (Codex and
// Claude Code) and everything that differs between them: the commands
// Huddle runs for each role, how it reads a run's session id and final
// message, how it spots a usage limit, and the tiny test run that checks a
// model and effort before they are saved.
//
// Any of the three roles (Dev, PM, companion) can be played by either CLI.
// The rest of Huddle asks this package instead of naming a CLI itself, so
// adding a CLI means changing this package only.
//
// The facts below were checked against codex-cli 0.160.0 and Claude Code
// 2.1.299; README.md lists them with the commands they produce.
package agents

import (
	"fmt"

	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// The CLI ids, as stored in the config file (agents.dev.cli and so on).
const (
	CodexID  = "codex"
	ClaudeID = "claude"
)

// CLI describes one agent command-line tool and how Huddle drives it.
type CLI struct {
	Name        string  // "Codex" or "Claude Code", shown in the UI
	ID          string  // "codex" or "claude", stored in the config
	Binary      string  // executable name, looked up on PATH
	SignIn      string  // the login Huddle's startup check requires, for /settings
	KnownModels []Model // offered by the wizard; Custom… allows any name
	Efforts     []string
	// DefaultEffort is the effort the wizard suggests. It is not what the
	// CLI uses when no effort is given: an empty effort in the config
	// leaves the flag out, and the CLI picks its own.
	DefaultEffort string
}

// Model is one entry in the wizard's model list.
type Model struct {
	Label string // shown, e.g. "Opus 5.5"
	Value string // passed to the CLI, e.g. "opus"
}

// Codex returns the description of OpenAI's Codex CLI.
//
// Neither CLI can list its models, so the known models are a starting list
// for the wizard, kept here so they are easy to update. Codex's efforts are
// the values its API accepted for gpt-6.1-sol on 2026-10-14 (an invalid one
// is answered with "Supported values are: 'none', 'minimal', 'low',
// 'medium', 'high', 'xhigh', and 'max'"). Not every model takes every
// effort; the validation run finds out.
func Codex() CLI {
	return CLI{
		Name:   "Codex",
		ID:     CodexID,
		Binary: "codex",
		SignIn: "ChatGPT",
		KnownModels: []Model{
			{Label: "GPT-6.1 Sol", Value: "gpt-6.1-sol"},
		},
		Efforts:       []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"},
		DefaultEffort: "medium",
	}
}

// Claude returns the description of Anthropic's Claude Code CLI.
//
// Claude Code's aliases always mean the newest model of their family, so
// they stay valid when a new version comes out; only the labels age. Its
// efforts are the ones `claude --help` lists for --effort.
func Claude() CLI {
	return CLI{
		Name:   "Claude Code",
		ID:     ClaudeID,
		Binary: "claude",
		SignIn: "claude.ai",
		KnownModels: []Model{
			{Label: "Opus 5.5", Value: "opus"},
			{Label: "Sonnet 5.5", Value: "sonnet"},
			{Label: "Haiku 5.5", Value: "haiku"},
			{Label: "Fable 5.1", Value: "fable"},
		},
		Efforts:       []string{"low", "medium", "high", "xhigh", "max"},
		DefaultEffort: "medium",
	}
}

// All returns every CLI Huddle knows, in the order the wizard lists them.
func All() []CLI {
	return []CLI{Codex(), Claude()}
}

// ByID returns the CLI whose ID is id.
func ByID(id string) (CLI, error) {
	for _, c := range All() {
		if c.ID == id {
			return c, nil
		}
	}
	return CLI{}, fmt.Errorf("unknown agent CLI %q (want %q or %q)", id, CodexID, ClaudeID)
}

// ShortName is the CLI's name in tight places such as the status bar
// badge: "Codex" or "Claude".
func (c CLI) ShortName() string {
	if c.ID == ClaudeID {
		return "Claude"
	}
	return c.Name
}

// ValidEffort reports whether effort is one of the CLI's effort levels. The
// empty effort is valid: it means the CLI's own default.
func (c CLI) ValidEffort(effort string) bool {
	if effort == "" {
		return true
	}
	for _, e := range c.Efforts {
		if e == effort {
			return true
		}
	}
	return false
}

// Choice is what plays one role: a CLI with a model and an effort. An
// empty Model or Effort means the CLI's own default, and its flag is left
// out of the command.
type Choice struct {
	CLI    CLI
	Model  string
	Effort string
}

// String describes the choice for people: "Claude Code · opus · high", or
// "Codex · CLI default model · CLI default effort".
func (c Choice) String() string {
	model, effort := c.Model, c.Effort
	if model == "" {
		model = "CLI default model"
	}
	if effort == "" {
		effort = "CLI default effort"
	}
	return c.CLI.Name + " · " + model + " · " + effort
}

// Team says what plays each of the three roles.
type Team struct {
	Dev       Choice
	PM        Choice
	Companion Choice
}

// For returns what plays role.
func (t Team) For(role session.Role) Choice {
	switch role {
	case session.PM:
		return t.PM
	case session.Companion:
		return t.Companion
	default:
		return t.Dev
	}
}

// RoleTitle names a working role with the CLI that plays it, for the
// screen and the control issue: "Dev (Codex)", "PM (Claude Code)".
func (t Team) RoleTitle(role session.Role) string {
	if role == session.PM {
		return "PM (" + t.PM.CLI.Name + ")"
	}
	return "Dev (" + t.Dev.CLI.Name + ")"
}
