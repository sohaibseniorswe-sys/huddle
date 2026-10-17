package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/publish"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
)

type draftReviewMsg struct {
	preview publish.Preview
	err     error
}
type draftPublishedMsg struct {
	result publish.Result
	err    error
}

// reviewDraft performs read-only checks, then presents exact specs/destination.
func (m Model) reviewDraft() (Model, tea.Cmd) {
	if m.cfg.Draft == nil {
		return m, nil
	}
	workspace, client := *m.cfg.Draft, m.cfg.Control.Client
	m = m.addEvent(m.huddleEvent("Checking the saved draft and publish gate…"))
	return m, func() tea.Msg {
		preview, err := publish.Review(workspace)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), githubTimeout)
			defer cancel()
			err = publish.GateReview(ctx, client, workspace, preview)
		}
		return draftReviewMsg{preview: preview, err: err}
	}
}

func (m Model) handleDraftReview(msg draftReviewMsg) Model {
	if msg.err != nil {
		return m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle, Text: "Cannot publish: " + msg.err.Error()})
	}
	d := msg.preview.Draft
	ms := start.PlannedMilestone{Title: d.Title, Description: d.README}
	ordered, err := d.Order()
	if err != nil {
		return m.addEvent(m.huddleEvent(err.Error()))
	}
	for _, is := range ordered {
		ms.Issues = append(ms.Issues, start.PlannedIssue{Title: is.Title, Body: is.Text})
	}
	m.planText = d.Text()
	return m.showCard(card{kind: cardPlan, plan: start.Plan{Milestones: []start.PlannedMilestone{ms}}, revision: msg.preview.Revision, target: msg.preview.Project.Repo + " @ " + msg.preview.Project.Branch})
}

// agreeDraft is reachable only through the reviewed card's Agree action.
func (m Model) agreeDraft(revision string) (Model, tea.Cmd) {
	workspace, client := *m.cfg.Draft, m.cfg.Control.Client
	m = m.addEvent(m.youEvent("Approved publishing the reviewed draft"))
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), planTimeout)
		defer cancel()
		result, err := publish.Publish(ctx, client, workspace, publish.Approval{Confirmed: true, Revision: revision})
		return draftPublishedMsg{result: result, err: err}
	}
}

func (m Model) handleDraftPublished(msg draftPublishedMsg) Model {
	if msg.err != nil {
		return m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle, Text: "Publish stopped: " + msg.err.Error() + ". Completed steps are journaled; review and approve again to retry."})
	}
	text := fmt.Sprintf("Published %s on %s @ %s: %d issues, control #%d is dev-ready. Reopen the project to start building.", msg.result.Title, msg.result.Repo, msg.result.Branch, len(msg.result.Issues), msg.result.Control)
	return m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle, Text: text})
}
