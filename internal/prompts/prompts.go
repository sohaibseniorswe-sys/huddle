// Package prompts embeds the agents' operating principles (MVP_PLAN.md
// section 9) and renders the launch prompt sent on every turn.
package prompts

import (
	_ "embed"
	"fmt"
	"strings"
)

var (
	//go:embed shared.md
	shared string
	//go:embed dev.md
	dev string
	//go:embed pm.md
	pm string
)

// Role is the agent a prompt is for: "dev" or "pm".
type Role string

const (
	Dev Role = "dev"
	PM  Role = "pm"
)

// Launch returns the launch prompt for role: the shared section, the role's
// section, then the turn line. agent names the CLI playing the role
// ("Codex" or "Claude Code"), so the agent knows which role it has whatever
// tool it is. Placeholders are filled with plain string replacement, so
// nothing else in the text is interpreted.
func Launch(role Role, agent, milestone string, controlIssue int) (string, error) {
	var section string
	switch role {
	case Dev:
		section = dev
	case PM:
		section = pm
	default:
		return "", fmt.Errorf("unknown role %q", role)
	}
	r := strings.NewReplacer("{{milestone}}", milestone, "{{control_issue}}", fmt.Sprint(controlIssue), "{{agent}}", agent)
	return r.Replace(strings.TrimRight(shared, "\n")) + "\n\n" +
		r.Replace(strings.TrimRight(section, "\n")) + "\n\n" +
		fmt.Sprintf("It's your turn. Read control issue #%d and continue.\n", controlIssue), nil
}
