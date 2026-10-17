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
// section, then the turn line. cli names the CLI playing the role ("Codex"
// or "Claude Code"), so the agent knows which role it has whatever tool it
// is. level is the autonomy level (0–10): the shared AUTONOMY section's
// table, read at that level, is the only rule for when an agent hands over.
// Placeholders are filled with plain string replacement, so nothing else in
// the text is interpreted.
func Launch(role Role, cli, milestone string, controlIssue, level int) (string, error) {
	var section string
	switch role {
	case Dev:
		section = dev
	case PM:
		section = pm
	default:
		return "", fmt.Errorf("unknown role %q", role)
	}
	r := strings.NewReplacer(
		"{{milestone}}", milestone,
		"{{control_issue}}", fmt.Sprint(controlIssue),
		"{{cli}}", cli,
		"{{level}}", fmt.Sprint(level),
	)
	return r.Replace(strings.TrimRight(shared, "\n")) + "\n\n" +
		r.Replace(strings.TrimRight(section, "\n")) + "\n\n" +
		fmt.Sprintf("It's your turn. Read control issue #%d and continue.\n", controlIssue), nil
}

// IssueLaunch scopes v4 work to one implementation issue in a fresh session.
// Same-role ready handoffs preserve autonomy without accumulating context or
// forcing a PM review between issues. Legacy Launch remains byte-for-byte v3.
func IssueLaunch(role Role, cli, milestone string, controlIssue, level int) (string, error) {
	prompt, err := Launch(role, cli, milestone, controlIssue, level)
	if err != nil {
		return "", err
	}
	scope := "V4 SESSION BOUNDARY\nThis is a fresh session. Reload the control issue and ALL comments, milestone issues, AGENTS.md, Session.md if present, and recent git history before acting. Do not rely on a previous transcript.\n"
	if role == Dev {
		scope += "Complete at most ONE unblocked implementation issue in this session, with the required tests, branch, PR and evidence. Persist the issue result and next issue in Session.md and the control comment. If independent work remains and your autonomy level does not call for a PM handoff, set state:dev-ready and end the session: Huddle starts a fresh Dev session for the next issue. A same-role ready label is a session boundary, not a PM handoff. Follow the autonomy table for actual PM handoffs; report Completed to the PM only when all implementation issues are closed.\n"
	} else {
		scope += "Handle this PM turn using the durable project state, then end the session with the appropriate ready/human/done label. Do not accumulate work in a continuing transcript.\n"
	}
	return prompt + "\n" + scope, nil
}
