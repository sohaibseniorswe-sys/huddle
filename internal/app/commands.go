package app

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/commands"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
)

// githubTimeout bounds one background GitHub request from the screen.
const githubTimeout = time.Minute

// runTyped runs the command typed in the text field when Enter is pressed
// in the menu. A visible command typed out in full runs; otherwise the
// highlighted command runs.
func (m Model) runTyped(matches []commands.Command) (Model, tea.Cmd) {
	name, args := commands.Split(m.input.Value())
	m = m.closeMenu()

	run := ""
	for _, c := range matches {
		if c.Name == name {
			run = name
		}
	}
	if run == "" && len(matches) > 0 {
		run, args = matches[min(m.menuSel, len(matches)-1)].Name, ""
	}
	if run == "" {
		return m.addEvent(m.huddleEvent(fmt.Sprintf("No command %s here; type / to see the commands", name))), nil
	}
	return m.runCommand(run, args)
}

// runCommand runs one slash command.
func (m Model) runCommand(name, args string) (Model, tea.Cmd) {
	switch name {
	case commands.Publish:
		return m.reviewDraft()
	case commands.Status:
		m.view, m.focus = viewStatus, focusActivity
		m.statusVP.GotoTop()
	case commands.Companion:
		m = m.toggleCompanion()
	case commands.Decide:
		return m.say(commands.Decide, decideMessage)
	case commands.Decision:
		if m.card.kind != cardNone {
			m.focus = focusCard
		}
	case commands.Pause:
		return m.runPause(), nil
	case commands.Resume:
		return m.runResume(), nil
	case commands.Log:
		m = m.openLatestLog()
	case commands.Settings:
		m = m.openSettings()
	case commands.Quit:
		return m.runQuit()
	}
	return m, nil
}

// runPause asks the loop to start no new turn. The running turn finishes
// normally: /pause never stops an agent.
func (m Model) runPause() Model {
	if m.paused {
		return m
	}
	m.cfg.Controls.Pause()
	m.paused = true
	m = m.addEvent(m.youEvent(commands.Pause))
	text := "Pausing: no new turn starts until /resume"
	if m.status.Phase == loop.PhaseWorking || m.status.Phase == loop.PhaseUsageWait {
		text = fmt.Sprintf("Pausing: the %s's current turn will finish, then nothing new starts", m.roleTitle(m.status.Role))
	}
	return m.addEvent(m.huddleEvent(text))
}

// runResume lets the loop start turns again. The loop reports "Resumed"
// itself when it starts the next turn.
func (m Model) runResume() Model {
	m.cfg.Controls.Resume()
	m.paused = false
	return m.addEvent(m.youEvent(commands.Resume))
}

// runQuit stops the loop: the running agent's process group is killed and
// the loop puts its ready label back, so the next start resumes it. The
// screen closes when the loop reports that it has ended (LoopDoneMsg).
func (m Model) runQuit() (Model, tea.Cmd) {
	if m.loopDone {
		return m, tea.Quit
	}
	if m.quitting {
		return m, nil
	}
	m.quitting = true
	m.focus = focusInput
	m = m.addEvent(m.youEvent(commands.Quit))
	m = m.addEvent(m.huddleEvent("Stopping: any running agent is stopped and its ready label put back"))
	m.cfg.Stop()
	if m.cfg.StopCompanion != nil {
		m.cfg.StopCompanion() // a companion answer is no longer wanted
	}
	return m, nil
}

// factsMsg is sent by the command fetchFacts returns, with the facts read
// from GitHub or the error that stopped it.
type factsMsg struct {
	facts gh.Facts
	err   error
}

// fetchFacts returns a command that reads the GitHub facts in the
// background. The caller sets m.fetching, so only one runs at a time.
func (m Model) fetchFacts() tea.Cmd {
	control, milestone, now := m.cfg.Control, m.cfg.Milestone, m.cfg.Now
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
		defer cancel()
		f, err := gh.FetchFacts(ctx, control.Client, control.Repo, milestone)
		f.FetchedAt = now()
		return factsMsg{facts: f, err: err}
	}
}

// handleFacts takes in a refresh. A failed refresh keeps the old facts.
func (m Model) handleFacts(msg factsMsg) Model {
	m.fetching = false
	m.lastFetch = m.cfg.Now()
	m.factsErr = msg.err
	if msg.err == nil {
		m.facts = msg.facts
	}
	return m
}
