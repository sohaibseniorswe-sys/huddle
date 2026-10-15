package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// This file is the /settings view: the project's settings, read-only, in
// the Activity slot (docs/v3/screens/09-settings.png). Changing them is the
// setup wizard's job (v3.4).

// settingsKeyWidth is the width of the names in the /settings view.
const settingsKeyWidth = 16

// settingsFooter closes the /settings view.
const settingsFooter = "Read-only. To change settings, quit and run: huddle setup"

// Settings is what /settings shows besides what Config already holds.
type Settings struct {
	Home     string // the home directory, to show paths as ~/...
	Path     string // the project's config file
	Exists   bool   // false: there is no file yet and the defaults are used
	Autonomy int
	// CompanionFollowsPM is the config's same_as_pm: the companion uses
	// the PM's CLI, model and effort.
	CompanionFollowsPM bool
	// Versions maps a CLI's ID to its installed version, "0.160.0".
	Versions map[string]string
	// Sessions and CompanionSessions are the session files, for the ids.
	Sessions          session.Store
	CompanionSessions session.Store
}

// openSettings shows /settings in the Activity slot and focuses it. Its
// text is drawn now, because it reads the logs.
func (m Model) openSettings() Model {
	m.settings = m.settingsContent()
	m.view, m.focus = viewSettings, focusActivity
	m.settingsVP.SetContent(m.settings)
	m.settingsVP.GotoTop()
	return m
}

// settingsContent draws the /settings view.
func (m Model) settingsContent() string {
	s := m.cfg.Settings
	section := func(name string) string { return ui.Fg(ui.Purple).Bold(true).Render(name) }
	kv := func(k, v string) string {
		return "  " + ui.Muted.Width(settingsKeyWidth).Render(k) + ui.Plain.Render(v)
	}
	more := func(v string) string { return kv("", v) } // a second line for the key above

	file := m.tilde(s.Path)
	if !s.Exists {
		file += "  (not written yet: Huddle uses the defaults)"
	}
	runs, _ := findRuns(m.cfg.LogDir, m.cfg.RepoDir) // unreadable logs only lose the counts
	companionRuns, _ := findRuns(filepath.Join(m.cfg.LogDir, "companion"), m.cfg.RepoDir)

	rows := []string{
		ui.Muted.Render(file),
		"",
		section("Project"),
		kv("Repo", m.cfg.Repo),
		kv("Folder", m.tilde(m.cfg.RepoDir)),
		kv("Control issue", fmt.Sprintf("#%d · %s", m.cfg.Control.Number, m.cfg.Milestone)),
		"",
		section("Agents"),
	}
	team := m.cfg.Team
	rows = append(rows,
		kv("Dev", m.cliLine(team.Dev.CLI)),
		more(choiceLine(team.Dev, lastModel(runs, "-dev.log", team.Dev.CLI))),
		kv("PM", m.cliLine(team.PM.CLI)),
		more(choiceLine(team.PM, lastModel(runs, "-pm.log", team.PM.CLI))),
	)
	if s.CompanionFollowsPM {
		rows = append(rows, kv("Companion", team.Companion.CLI.Name+" (follows the PM)"))
	} else {
		rows = append(rows,
			kv("Companion", m.cliLine(team.Companion.CLI)),
			more(choiceLine(team.Companion, lastModel(companionRuns, ".log", team.Companion.CLI))))
	}
	rows = append(rows,
		"",
		section("Autonomy"),
		kv("Level", autonomyText(s.Autonomy)),
		"",
		section("Files"),
		kv("Logs", fmt.Sprintf("%s/  (%s of this repo)", m.tilde(m.cfg.LogDir), plural(len(runs), "run"))),
		kv("Sessions", m.sessionsLine()),
		"",
		ui.Muted.Render(settingsFooter),
	)
	return strings.Join(rows, "\n")
}

// cliLine is a CLI's row: "Codex 0.160.0 · signed in with ChatGPT". The
// startup checks have made sure of the login.
func (m Model) cliLine(cli agents.CLI) string {
	version := m.cfg.Settings.Versions[cli.ID]
	if version == "" {
		version = "(version unknown)"
	}
	return fmt.Sprintf("%s %s · signed in with %s", cli.Name, version, cli.SignIn)
}

// choiceLine is the model and effort under a CLI's row: "model opus ·
// effort high". When the model is the CLI's default, or an alias, it adds
// the model the role's last run really used, as the CLI reported it.
func choiceLine(c agents.Choice, used string) string {
	model := c.Model
	switch {
	case model == "" && used != "":
		model = "CLI default (" + used + " in the last run)"
	case model == "":
		model = "CLI default (not run yet)"
	case used != "" && used != model:
		model += " (" + used + " in the last run)"
	}
	effort := c.Effort
	if effort == "" {
		effort = "CLI default"
	}
	return "model " + model + " · effort " + effort
}

// lastModel returns the model that the newest run in runs (oldest first)
// whose log name ends in suffix used, as cli reported it; "" if there is no
// such run of cli, or it did not say.
func lastModel(runs []logRun, suffix string, cli agents.CLI) string {
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		if !strings.HasSuffix(r.path, suffix) {
			continue
		}
		if r.agent != cli.Name {
			return "" // the role was played by the other CLI then
		}
		if cli.ID == agents.CodexID {
			// Codex names its model in the header at the top of the log.
			head, err := readHead(r.path)
			if err != nil {
				return ""
			}
			return agents.CodexModel(runner.StripStamps(head))
		}
		// Claude Code's envelope is the last output line of the log.
		lines := strings.Split(runner.StripStamps(readTail(r.path, maxPMLogTail)), "\n")
		for j := len(lines) - 1; j >= 0; j-- {
			if env, err := agents.ParseClaude(lines[j]); err == nil {
				return env.Model()
			}
		}
		return ""
	}
	return ""
}

// readHead reads the first few kilobytes of the file at path, where Codex
// prints its header.
func readHead(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, err := f.Read(buf)
	if n == 0 && err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

// sessionsLine is the session ids, shortened: "Dev 01a1…f981 · PM 717c…f5c3".
func (m Model) sessionsLine() string {
	ids := m.cfg.Settings.Sessions.Load()
	companionID := m.cfg.Settings.CompanionSessions.Load().Get(session.Companion)
	var parts []string
	for _, p := range []struct{ name, id string }{{"Dev", ids.Dev}, {"PM", ids.PM}, {"Companion", companionID}} {
		if p.id != "" {
			parts = append(parts, p.name+" "+shortID(p.id))
		}
	}
	if len(parts) == 0 {
		return "none yet"
	}
	return strings.Join(parts, " · ")
}

// shortID shortens a session id to its first and last four characters.
func shortID(id string) string {
	if len(id) <= 10 {
		return id
	}
	return id[:4] + "…" + id[len(id)-4:]
}

// tilde shortens a path under the home directory to start with "~".
func (m Model) tilde(path string) string {
	home := m.cfg.Settings.Home
	if home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

// plural is "1 run" or "12 runs".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// autonomyText is "8 · Autonomous", the level and its band's name.
func autonomyText(level int) string {
	return fmt.Sprintf("%d · %s", level, config.AutonomyName(level))
}
