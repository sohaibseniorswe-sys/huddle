package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
)

// This file is plan mode (docs/v3/issues/06-start-modes.md): the human
// plans milestones with the companion, the companion writes the agreed
// plan as a <plan> block, the run screen shows it as a card, and on Agree
// Huddle creates the milestones, issues and control issues on GitHub and
// queues them. The loop goroutine (see run.go) then runs the queue.
//
// A plan can also be agreed while a milestone runs: its milestones then
// wait in the queue until the running one is done.

// planTimeout bounds creating a whole plan on GitHub.
const planTimeout = 10 * time.Minute

// planNote is the plan card's dim line.
const planNote = "On Agree, Huddle creates them on GitHub and runs them one after another."

// planTitles lists a plan's milestones with their issue counts:
// "M4 (5 issues), M5 (4 issues)".
func planTitles(p start.Plan) string {
	var parts []string
	for _, ms := range p.Milestones {
		n := fmt.Sprintf("%d issues", len(ms.Issues))
		if len(ms.Issues) == 1 {
			n = "1 issue"
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", ms.Title, n))
	}
	return strings.Join(parts, ", ")
}

// takePlan handles a <plan> block in a companion reply. A plan that does
// not parse is never shown as a card: the human sees why in the
// conversation, and nothing is created.
func (m Model) takePlan(text string) Model {
	p, err := start.ParsePlan(text)
	if err != nil {
		return m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle,
			Text: "The companion's plan cannot be used: " + err.Error() + ". Nothing was created. Ask the companion to write the plan again."})
	}
	return m.showCard(card{kind: cardPlan, plan: p})
}

// showPlan shows the whole plan in the Activity slot (Show all) and
// focuses it, so it can be scrolled. Esc goes back to the Activity list;
// the card keeps waiting.
func (m Model) showPlan(p start.Plan) Model {
	m.planText = p.Text()
	m.view, m.focus = viewPlan, focusActivity
	m = m.sync()
	m.planVP.GotoTop()
	return m
}

// planAppliedMsg is sent by the command applyPlan returns when Huddle has
// created the plan on GitHub, or stopped at an error.
type planAppliedMsg struct {
	created []start.Created
	err     error
}

// agreePlan starts creating p on GitHub.
func (m Model) agreePlan(p start.Plan) (Model, tea.Cmd) {
	m = m.addEvent(m.youEvent("Agreed: plan, " + p.Summary()))
	m = m.addEvent(m.huddleEvent("Creating the milestones, issues and control issues on GitHub…"))
	if m.view == viewPlan {
		m.view = viewList
	}
	return m, m.applyPlan(p)
}

// applyPlan returns the command that creates p on GitHub. Each step is
// sent to the screen as an Activity event while the next one runs:
// cfg.Send is Program.Send, which may be called from the command's
// goroutine. Without cfg.Send (in tests) the steps are not shown.
func (m Model) applyPlan(p start.Plan) tea.Cmd {
	client, repo, send, now := m.cfg.Control.Client, m.cfg.Repo, m.cfg.Send, m.cfg.Now
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), planTimeout)
		defer cancel()
		report := func(step string) {
			if send != nil {
				send(EventMsg(activity.Event{Time: now(), Actor: activity.ActorHuddle, Text: step}))
			}
		}
		created, err := start.Apply(ctx, client, repo, p, report)
		return planAppliedMsg{created: created, err: err}
	}
}

// handlePlanApplied queues the control issues Huddle created. In plan
// mode it also asks the loop goroutine to start the first of them; while a
// milestone runs, they wait for it.
func (m Model) handlePlanApplied(msg planAppliedMsg) Model {
	var controls []int
	for _, c := range msg.created {
		controls = append(controls, c.Control)
	}
	if msg.err != nil {
		m = m.addEvent(m.huddleEvent("Could not create the whole plan: " + msg.err.Error() +
			". What the lines above list exists on GitHub; nothing was removed. Talk to the companion and agree a plan for the rest."))
	}
	if len(controls) == 0 {
		return m
	}
	if err := m.cfg.Queue.Add(controls...); err != nil {
		return m.addEvent(m.huddleEvent("Could not queue the milestones: " + err.Error()))
	}
	m.queued = m.loadQueue()
	m = m.addEvent(m.huddleEvent("Queued control issues " + issueNumbers(controls) + "; they run one after another"))
	if m.planning && m.cfg.Begin != nil {
		m.cfg.Begin()
	}
	return m
}

// issueNumbers is "#12, #14".
func issueNumbers(numbers []int) string {
	parts := make([]string, len(numbers))
	for i, n := range numbers {
		parts[i] = fmt.Sprintf("#%d", n)
	}
	return strings.Join(parts, ", ")
}

// loadQueue reads the milestone queue. The file is small and local, so
// reading it in Update is fine; it is read only when the queue may have
// changed. An unreadable queue shows as empty here: the loop goroutine
// reports the error when it needs the queue.
func (m Model) loadQueue() []int {
	if m.cfg.Queue.Path == "" {
		return nil
	}
	queued, err := m.cfg.Queue.Load()
	if err != nil {
		return nil
	}
	return queued
}

// handleMilestone takes in a milestone that has just started: the first
// one, or the next one from the queue. The project pane, the facts and
// /settings switch to it. The companion keeps its conversation.
func (m Model) handleMilestone(msg MilestoneMsg) (Model, tea.Cmd) {
	m.planning, m.finished = false, false
	m.cfg.Milestone, m.cfg.Control = msg.Milestone, msg.Control
	m.cfg.Settings.Sessions = session.New(m.cfg.Settings.Home, m.cfg.Repo, msg.Control.Number)
	m.started = m.cfg.Now()
	m.facts, m.factsErr = gh.Facts{}, nil
	m.queued = m.loadQueue()
	m = m.addEvent(m.huddleEvent(fmt.Sprintf("Started %s · control #%d · autonomy %d", msg.Milestone, msg.Control.Number, m.cfg.Settings.Autonomy)))
	if m.fetching {
		return m, nil // the refresh under way is for the old milestone; the next tick fetches again
	}
	m.fetching = true
	return m, m.fetchFacts()
}

// queueText is the Queue row of /status: "#58 Milestone 3 now · next
// #61, #64". It is "" when nothing is queued.
func (m Model) queueText() string {
	var waiting []int
	for _, n := range m.queued {
		if m.planning || n != m.cfg.Control.Number {
			waiting = append(waiting, n)
		}
	}
	if len(waiting) == 0 {
		return ""
	}
	now := fmt.Sprintf("#%d %s now", m.cfg.Control.Number, m.cfg.Milestone)
	if m.planning {
		now = "planning"
	}
	return now + " · next " + issueNumbers(waiting)
}
