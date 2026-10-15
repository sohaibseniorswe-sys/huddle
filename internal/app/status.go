package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// statusKeyWidth is the width of the names in the /status view.
const statusKeyWidth = 14

// maxIssueBar is the most cells the issue bar in /status draws; a bigger
// milestone is scaled down to it.
const maxIssueBar = 30

// roleTitle names a role with the CLI that plays it, short enough for the
// badge: "Dev (Codex)", "PM (Claude)".
func (m Model) roleTitle(r session.Role) string {
	if r == session.PM {
		return "PM (" + m.cfg.Team.PM.CLI.ShortName() + ")"
	}
	return "Dev (" + m.cfg.Team.Dev.CLI.ShortName() + ")"
}

// roleShort names a role alone.
func roleShort(r session.Role) string {
	if r == session.PM {
		return "PM"
	}
	return "Dev"
}

// badge returns the status bar's badge text and colour.
func (m Model) badge() (string, lipgloss.Color) {
	s := m.status
	switch {
	case m.quitting:
		return "Stopping", ui.Amber
	case s.Phase == loop.PhaseWorking:
		return m.roleTitle(s.Role) + " working", ui.Blue
	case s.Phase == loop.PhaseUsageWait:
		return "Waiting for usage reset · " + s.Until.Format("15:04"), ui.Amber
	case s.Phase == loop.PhasePaused:
		return "Paused · " + roleShort(s.Role) + " next", ui.Amber
	case s.Phase == loop.PhaseHuman, s.Phase == loop.PhaseNoLabel:
		return "Waiting for you", ui.Amber
	case s.Phase == loop.PhaseDone:
		return "Done", ui.Green
	default:
		return "Starting", ui.Dim
	}
}

// infoParts are the parts of the info line below the status bar.
func (m Model) infoParts() []string {
	kv := func(k, v string) string { return ui.Muted.Render(k+" ") + v }
	return []string{
		kv("Issues", ui.Plain.Render(m.issuesShort())),
		kv("PRs", ui.Plain.Render(m.prsShort())),
		kv("CI", m.mainCI()),
		kv("Usage", ui.Plain.Render(m.usage(false))),
		kv("Elapsed", ui.Plain.Render(elapsed(m.now.Sub(m.started)))),
	}
}

// factsPending reports whether no GitHub facts have arrived yet.
func (m Model) factsPending() bool { return m.facts.FetchedAt.IsZero() }

// issuesShort is "4/7 closed".
func (m Model) issuesShort() string {
	if m.factsPending() {
		return "…"
	}
	return fmt.Sprintf("%d/%d closed", len(m.facts.Closed()), len(m.facts.Issues))
}

// prsShort is "#73 open", "#73 #74 open" or "none open".
func (m Model) prsShort() string {
	if m.factsPending() {
		return "…"
	}
	if len(m.facts.OpenPRs) == 0 {
		return "none open"
	}
	var nums []string
	for _, pr := range m.facts.OpenPRs {
		nums = append(nums, fmt.Sprintf("#%d", pr.Number))
	}
	return strings.Join(nums, " ") + " open"
}

// mainCI is CI on main, coloured: green, red or running.
func (m Model) mainCI() string {
	if m.factsPending() {
		return ui.Plain.Render("…")
	}
	ci := m.facts.MainCI()
	switch ci {
	case "green":
		return ui.Fg(ui.Green).Render(ci)
	case "red":
		return ui.Fg(ui.Red).Render(ci)
	case "running":
		return ui.Fg(ui.Amber).Render(ci)
	}
	return ui.Plain.Render(ci)
}

// usage is each working CLI's usage state, "Codex ok · Claude ok", or
// with the time a CLI waits for. Usage limits belong to a CLI's account,
// so a CLI that plays both the Dev and the PM is named once. long names
// the CLIs in full ("Claude Code"), for /status.
func (m Model) usage(long bool) string {
	var parts []string
	seen := map[string]bool{}
	for _, role := range []session.Role{session.Dev, session.PM} {
		cli := m.cfg.Team.For(role).CLI
		if seen[cli.ID] {
			continue
		}
		seen[cli.ID] = true
		name := cli.ShortName()
		if long {
			name = cli.Name
		}
		state := " ok"
		waiting := m.status.Phase == loop.PhaseUsageWait && m.cfg.Team.For(m.status.Role).CLI.ID == cli.ID
		if waiting {
			state = " waiting until " + m.status.Until.Format("15:04")
		}
		parts = append(parts, name+state)
	}
	return strings.Join(parts, " · ")
}

// elapsed formats a duration as "2h14m" or "14m".
func elapsed(d time.Duration) string {
	d = d.Truncate(time.Minute)
	if h := int(d.Hours()); h > 0 {
		return fmt.Sprintf("%dh%02dm", h, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// statusContent draws the /status view: facts from GitHub and from
// Huddle's own state. It never starts an agent.
func (m Model) statusContent() string {
	kv := func(k, v string) string { return ui.Muted.Width(statusKeyWidth).Render(k) + v }
	plain := ui.Plain.Render
	usage := m.usage(true)
	if m.status.Phase != loop.PhaseUsageWait {
		usage += " · no waits"
	}
	rows := []string{
		kv("Turn", plain(m.turnText())),
		kv("Issues", m.issuesText()),
		kv("Open PRs", plain(m.openPRsText())),
		kv("Main", m.mainText()),
		kv("Decisions", plain(m.decisionsText())),
		kv("Autonomy", plain(autonomyText(m.cfg.Settings.Autonomy))),
		kv("Usage", plain(usage)),
		kv("Elapsed", plain(elapsed(m.now.Sub(m.started))+" · started "+m.started.Format("15:04"))),
		"",
		ui.Muted.Render("Answered by Huddle from GitHub and its logs. No agent was started."),
	}
	if m.factsErr != nil {
		rows = append(rows, ui.Muted.Render("Last GitHub refresh failed: "+m.factsErr.Error()))
	} else if !m.factsPending() {
		rows = append(rows, ui.Muted.Render("GitHub facts from "+m.facts.FetchedAt.Format("15:04:05")+"."))
	}
	return strings.Join(rows, "\n")
}

// turnText is the Turn row of /status.
func (m Model) turnText() string {
	s := m.status
	turn := fmt.Sprintf("turn %d", s.Turn)
	switch s.Phase {
	case loop.PhaseWorking:
		return fmt.Sprintf("%s · working %s · %s", m.roleTitle(s.Role), elapsed(m.now.Sub(s.Since)), turn)
	case loop.PhaseUsageWait:
		return fmt.Sprintf("%s · waiting for its usage limit until %s · %s", m.roleTitle(s.Role), s.Until.Format("15:04"), turn)
	case loop.PhasePaused:
		return fmt.Sprintf("Paused · %s next · %s", m.roleTitle(s.Role), turn)
	case loop.PhaseHuman:
		return fmt.Sprintf("Waiting for you since %s · %s", s.Since.Format("15:04"), turn)
	case loop.PhaseNoLabel:
		return fmt.Sprintf("No state label on #%d · add %s to start", m.cfg.Control.Number, gh.StateDevReady)
	case loop.PhaseDone:
		return fmt.Sprintf("Done · %d turns", s.Turn)
	}
	return "Starting"
}

// issuesText is the Issues row of /status: a bar with a cell per issue,
// green when closed, and the closed issues' numbers.
func (m Model) issuesText() string {
	if m.factsPending() {
		return ui.Plain.Render("loading…")
	}
	total, closed := len(m.facts.Issues), m.facts.Closed()
	if total == 0 {
		return ui.Plain.Render("no issues in this milestone")
	}
	cells, filled := total, len(closed)
	if total > maxIssueBar {
		cells, filled = maxIssueBar, len(closed)*maxIssueBar/total
	}
	bar := ui.Fg(ui.Green).Render(strings.Repeat("■", filled)) + ui.Fg(ui.Faint).Render(strings.Repeat("■", cells-filled))
	text := fmt.Sprintf("  %d of %d closed", len(closed), total)
	if len(closed) > 0 {
		var nums []string
		for _, n := range closed {
			nums = append(nums, fmt.Sprintf("#%d", n))
		}
		text += "  (" + strings.Join(nums, " ") + ")"
	}
	return bar + ui.Plain.Render(text)
}

// openPRsText is the Open PRs row of /status.
func (m Model) openPRsText() string {
	if m.factsPending() {
		return "loading…"
	}
	if len(m.facts.OpenPRs) == 0 {
		return "none"
	}
	var parts []string
	for _, pr := range m.facts.OpenPRs {
		parts = append(parts, fmt.Sprintf("#%d %s · %s", pr.Number, pr.Title, checksText(pr.Checks)))
	}
	return strings.Join(parts, "; ")
}

// checksText describes a pull request's CI.
func checksText(c gh.Checks) string {
	switch {
	case c.Total == 0:
		return "no CI checks"
	case c.Pending > 0:
		return fmt.Sprintf("CI running (%d of %d checks)", c.Done, c.Total)
	case c.Failed > 0:
		return fmt.Sprintf("CI failed (%d of %d checks)", c.Failed, c.Total)
	}
	return "CI green"
}

// mainText is the Main row of /status.
func (m Model) mainText() string {
	if m.factsPending() {
		return ui.Plain.Render("loading…")
	}
	text := ""
	if pr := m.facts.LastMerged; pr != nil {
		text = fmt.Sprintf(" · last merge %s, PR #%d", pr.MergedAt.In(m.now.Location()).Format("15:04"), pr.Number)
	}
	return m.mainCI() + ui.Plain.Render(text)
}
