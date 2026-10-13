// Package commands is the list of slash commands of the run screen: their
// names and descriptions, when each one is shown, and how typing filters
// them. It decides nothing about what a command does; the run screen
// (package app) runs them.
package commands

import "strings"

// The command names.
const (
	Status    = "/status"
	Companion = "/companion"
	Pause     = "/pause"
	Resume    = "/resume"
	Reply     = "/reply"
	Log       = "/log"
	Quit      = "/quit"
)

// Command is one slash command.
type Command struct {
	Name string // "/status"
	Desc string // one short line for the menu
	// Args is true if text follows the name, as in "/reply <text>". Tab
	// then completes the name with a space after it.
	Args bool
	// Lit is true if the menu marks the command with ● because what it
	// toggles is on (/companion while the pane is open).
	Lit bool
}

// State is what the visibility rules look at.
type State struct {
	CompanionOpen bool // the Companion pane is open
	AgentActive   bool // an agent is running or about to run
	Paused        bool // /pause was used and no /resume yet
	HumanRequired bool // the control issue says state:human-required
}

// Visible returns the commands that make sense in state s, in menu order.
func Visible(s State) []Command {
	var out []Command
	for _, c := range all(s) {
		if shown(c.Name, s) {
			out = append(out, c)
		}
	}
	return out
}

// all returns every command in menu order. It is a function, not a
// package variable, so no code can change the list by accident.
func all(s State) []Command {
	companion := Command{Name: Companion, Desc: "open or close the companion pane"}
	if s.CompanionOpen {
		companion = Command{Name: Companion, Desc: "open or close the companion pane (open now)", Lit: true}
	}
	return []Command{
		{Name: Status, Desc: "facts: issues, PRs, CI, turns, usage, elapsed"},
		companion,
		{Name: Pause, Desc: "let the current turn finish, start no new one"},
		{Name: Resume, Desc: "start the next turn now"},
		{Name: Reply, Desc: "answer the PM on the control issue: /reply <text>", Args: true},
		{Name: Log, Desc: "raw output of every agent run, scrollable"},
		{Name: Quit, Desc: "stop the running agent now; it resumes on next start"},
	}
}

// shown is the visibility table from docs/v3/README.md.
func shown(name string, s State) bool {
	switch name {
	case Pause:
		return s.AgentActive && !s.Paused
	case Resume:
		return s.Paused
	case Reply:
		// Temporary until v3.2 brings the companion and decision cards.
		return s.HumanRequired
	default:
		return true
	}
}

// Filter returns the commands in cmds that match what is typed in the text
// field, for example "/s" or "/reply yes, go ahead". Before a space, every
// command whose name starts with the typed word matches. Once a space
// follows the word (arguments are being typed), only that exact command
// matches.
func Filter(cmds []Command, typed string) []Command {
	word, _, hasArgs := strings.Cut(typed, " ")
	var out []Command
	for _, c := range cmds {
		if hasArgs && c.Name == word || !hasArgs && strings.HasPrefix(c.Name, word) {
			out = append(out, c)
		}
	}
	return out
}

// Split splits typed text into the command name and its argument text:
// "/reply  yes, go" gives "/reply" and "yes, go".
func Split(typed string) (name, args string) {
	name, args, _ = strings.Cut(strings.TrimSpace(typed), " ")
	return name, strings.TrimSpace(args)
}
