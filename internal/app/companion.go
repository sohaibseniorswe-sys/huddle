package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// This file is the companion's side of the run screen: sending the human's
// messages, building the context block that goes with each one, and
// showing the replies.
//
// A companion run takes seconds or minutes. It runs in a tea.Cmd, which
// Bubble Tea runs on another goroutine, and its reply comes back to Update
// as a companionReplyMsg. Meanwhile Update keeps handling keys and the
// loop's reports, and the loop keeps running the Dev and PM on its own
// goroutine: neither ever waits for the companion. Only one companion run
// goes at a time, because each resumes the session the one before saved;
// messages sent meanwhile wait in m.queue and go together in the next run.

// decideMessage is what /decide asks the companion.
const decideMessage = "Draft the decision we have agreed, as a <decision> block."

// maxPMLogTail is how much of the end of the PM's last log is read to find
// its final message: the JSON envelope is the log's last output line.
const maxPMLogTail = 256 * 1024

// companionReplyMsg is sent by the command askCompanion returns, when the
// companion has answered or failed.
type companionReplyMsg struct {
	reply string
	err   error
}

// conversation reports whether the human and the companion have talked,
// which is when /decide makes sense.
func (m Model) conversation() bool {
	for _, e := range m.chat {
		if e.Who == companion.WhoYou || e.Who == companion.WhoCompanion {
			return true
		}
	}
	return false
}

// toggleCompanion opens or closes the Companion pane. Closing only hides
// it: the conversation is kept.
func (m Model) toggleCompanion() Model {
	if m.companionOpen {
		m.companionOpen = false
		if m.focus == focusCompanion {
			m.focus = focusInput
		}
		return m
	}
	return m.openCompanion()
}

// openCompanion shows the Companion pane, scrolled to the newest message.
func (m Model) openCompanion() Model {
	if m.companionOpen {
		return m
	}
	m.companionOpen = true
	m = m.sync()
	m.chatVP.GotoBottom()
	return m
}

// say sends message to the companion; shown is what the conversation shows
// for it (the same text, or "/decide"). The pane opens. If the companion is
// still answering an earlier message, this one waits for it.
func (m Model) say(shown, message string) (Model, tea.Cmd) {
	m = m.openCompanion()
	m = m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoYou, Text: shown})
	if m.cfg.Companion == nil {
		return m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle, Text: "No companion is set up in this Huddle."}), nil
	}
	m.queue = append(m.queue, message)
	if m.thinking {
		return m, nil
	}
	return m.askNext()
}

// askAboutPMQuestion opens the Companion pane and asks the companion to
// explain the PM's question, after the PM set state:human-required. The
// question itself is read from the control issue by the command.
func (m Model) askAboutPMQuestion() (Model, tea.Cmd) {
	m = m.openCompanion()
	if m.cfg.Companion == nil {
		return m, nil
	}
	m.queuePM = true
	m = m.addChat(companion.Entry{Time: m.cfg.Now(), Who: companion.WhoHuddle,
		Text: fmt.Sprintf("The PM set %s and waits for you. Asking the companion to explain its question…", gh.StateHumanRequired)})
	if m.thinking {
		return m, nil
	}
	return m.askNext()
}

// askNext starts a companion run with everything queued.
func (m Model) askNext() (Model, tea.Cmd) {
	message, pm := strings.Join(m.queue, "\n\n"), m.queuePM
	m.queue, m.queuePM = nil, false
	m.thinking = true
	m = m.followChat()
	// The spinner's Tick starts its animation; see the spinner.TickMsg case
	// in Update.
	return m, tea.Batch(m.askCompanion(message, pm), m.spin.Tick)
}

// handleCompanionReply shows a reply, or why there is none, and starts
// the next run if messages are waiting.
func (m Model) handleCompanionReply(msg companionReplyMsg) (Model, tea.Cmd) {
	m.thinking = false
	now := m.cfg.Now()
	if msg.err != nil {
		if !m.quitting {
			m = m.addChat(companion.Entry{Time: now, Who: companion.WhoHuddle, Text: "The companion could not answer: " + msg.err.Error()})
		}
	} else {
		r := companion.Parse(msg.reply)
		text := r.Text
		if text == "" {
			text = "(see the card below)"
		}
		m = m.addChat(companion.Entry{Time: now, Who: companion.WhoCompanion, Text: text})
		switch {
		case r.Decision != "":
			m = m.showCard(card{kind: cardDecision, text: r.Decision, reason: r.Reason})
		case r.Action != "":
			m = m.showCard(card{kind: cardAction, text: r.Action})
		}
	}
	if m.quitting || len(m.queue) == 0 && !m.queuePM {
		return m.followChat(), nil
	}
	return m.askNext()
}

// addChat adds e to the conversation and its file, and keeps the pane at
// the newest message if it was there. Writing one line to a local file is
// quick, so it happens here in Update; doing it here also keeps the
// entries in the order the human saw them.
func (m Model) addChat(e companion.Entry) Model {
	m.chat = append(m.chat, e)
	if m.cfg.History.Path != "" {
		if err := m.cfg.History.Append(e); err != nil && !m.historyErr {
			m.historyErr = true // reported once, not on every message
			m = m.addEvent(m.huddleEvent("Could not save the companion conversation: " + err.Error()))
		}
	}
	return m.followChat()
}

// followChat redraws the conversation and, if the pane was scrolled to the
// bottom, keeps it there.
func (m Model) followChat() Model {
	follow := m.chatVP.AtBottom()
	m = m.sync()
	if follow {
		m.chatVP.GotoBottom()
	}
	return m
}

// companionKey handles a key for the Companion pane: it scrolls.
func (m Model) companionKey(k tea.KeyMsg) Model {
	switch k.Type {
	case tea.KeyTab:
		m.focus = m.nextFocus()
		return m
	case tea.KeyEsc:
		m.focus = focusInput
		return m
	}
	if m, ok := m.slashOpensMenu(k); ok {
		return m
	}
	scroll(&m.chatVP, k)
	return m
}

// companionPane draws the Companion pane.
func (m Model) companionPane(rows int) string {
	p := ui.Pane{
		Title:   "Companion",
		Width:   m.width,
		Height:  rows,
		Focused: m.focus == focusCompanion,
		Accent:  ui.Amber,
		Body:    m.chatVP.View(),
	}
	if p.Focused {
		p.Hint = "/companion to close"
	}
	return p.Render()
}

// chatContent draws the conversation at width, with the spinner below it
// while the companion thinks.
func (m Model) chatContent(width int) string {
	var lines []ui.ChatLine
	for _, e := range m.chat {
		text := e.Text
		if e.Who == companion.WhoDecision {
			text = "agreed and posted: " + e.Text
		}
		lines = append(lines, ui.ChatLine{Who: e.Who, Text: text})
	}
	out := ui.Chat(width, lines)
	if len(lines) == 0 {
		out = ui.Muted.Render("Ask the companion anything about this run. It reads everything and changes nothing.")
	}
	if m.thinking {
		out += "\n\n" + m.spin.View() + ui.Muted.Render(" companion is thinking…")
	}
	return out
}

// askCompanion returns the command that runs the companion with message.
// With pmQuestion, the message starts with the PM's latest comment on the
// control issue (the question it set state:human-required for).
//
// Everything the context block needs from the model is copied here, in
// Update; the command reads GitHub and the log files itself, off the
// Update goroutine.
func (m Model) askCompanion(message string, pmQuestion bool) tea.Cmd {
	info := m.companionContext()
	control, logDir, repoDir := m.cfg.Control, m.cfg.LogDir, m.cfg.RepoDir
	agentRunning := m.status.Phase == loop.PhaseWorking
	talker := m.cfg.Companion
	ctx := m.cfg.CompanionCtx
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		ghCtx, cancel := context.WithTimeout(ctx, githubTimeout)
		comments, err := control.Client.Comments(ghCtx, control.Repo, control.Number)
		cancel()
		if err != nil {
			info.Comments = []string{"could not read the control issue: " + err.Error()}
		} else {
			info.Comments = formatComments(comments[max(len(comments)-companion.MaxComments, 0):])
		}
		if pmQuestion {
			message = strings.TrimSpace(pmQuestionMessage(comments, control.Number) + "\n\n" + message)
		}
		runs, _ := findRuns(logDir, repoDir) // no runs is fine: those parts say "none"
		info.PMOutput = lastPMOutput(runs)
		if agentRunning && len(runs) > 0 {
			info.AgentOutput = logTail(runs[len(runs)-1].path, companion.AgentOutputLines)
		}
		reply, err := talker.Ask(ctx, info.String(), message)
		return companionReplyMsg{reply: reply, err: err}
	}
}

// companionContext fills in the parts of the context block the model
// knows: the screen, the state, the milestone, the events and the card.
func (m Model) companionContext() companion.Context {
	c := companion.Context{
		Screen:      m.describeScreen(),
		State:       m.stateLine(),
		Milestone:   m.milestoneLine(),
		PendingCard: m.card.describe(),
	}
	for _, e := range m.events[max(len(m.events)-companion.MaxEvents, 0):] {
		c.Events = append(c.Events, e.Time.Format(eventTimeLayout)+" "+e.Actor+" "+e.Text)
	}
	return c
}

// describeScreen says what the human sees in the Activity slot.
func (m Model) describeScreen() string {
	var s string
	switch m.view {
	case viewSelect:
		e := m.events[m.selected]
		s = fmt.Sprintf("Activity list, line selected: %s %s %q", e.Time.Format(eventTimeLayout), e.Actor, e.Text)
	case viewStatus:
		s = "Status view (/status)"
	case viewSettings:
		s = "Settings view (/settings, read-only)"
	case viewLog:
		s = m.log.describe()
	default:
		s = "Activity list"
	}
	if m.focus == focusCard {
		s += "; the human is answering the card"
	}
	return s
}

// stateLine describes the loop's state:
// "state:dev-working, Dev (Codex) running for 12m, turn 5".
func (m Model) stateLine() string {
	s := m.status
	turn := fmt.Sprintf("turn %d", s.Turn)
	var line string
	switch s.Phase {
	case loop.PhaseWorking:
		label := gh.StateDevWorking
		if s.Role == session.PM {
			label = gh.StatePMWorking
		}
		line = fmt.Sprintf("%s, %s running for %s, %s", label, m.roleTitle(s.Role), elapsed(m.now.Sub(s.Since)), turn)
	case loop.PhaseUsageWait:
		line = fmt.Sprintf("%s waiting for its usage limit until %s, %s", m.roleTitle(s.Role), s.Until.Format("15:04"), turn)
	case loop.PhasePaused:
		line = fmt.Sprintf("paused by the human, %s next, %s", m.roleTitle(s.Role), turn)
	case loop.PhaseHuman:
		line = fmt.Sprintf("%s, waiting for the human since %s, %s", gh.StateHumanRequired, s.Since.Format("15:04"), turn)
	case loop.PhaseNoLabel:
		line = fmt.Sprintf("no state label on #%d", m.cfg.Control.Number)
	case loop.PhaseDone:
		line = fmt.Sprintf("the milestone is finished (%s or control issue closed), %d turns", gh.StateDone, s.Turn)
	default:
		line = "starting"
	}
	if m.paused && s.Phase != loop.PhasePaused {
		line += ", pause requested (no new turn after this one)"
	}
	return line
}

// milestoneLine describes the milestone's progress:
// "Milestone 3, 4 of 7 issues closed (#59 #60 closed; #62 open)".
func (m Model) milestoneLine() string {
	if m.factsPending() {
		return m.cfg.Milestone + " (issues not loaded yet)"
	}
	var closed, open []string
	for _, is := range m.facts.Issues {
		if is.Closed {
			closed = append(closed, fmt.Sprintf("#%d", is.Number))
		} else {
			open = append(open, fmt.Sprintf("#%d", is.Number))
		}
	}
	var parts []string
	if len(closed) > 0 {
		parts = append(parts, strings.Join(closed, " ")+" closed")
	}
	if len(open) > 0 {
		parts = append(parts, strings.Join(open, " ")+" open")
	}
	line := fmt.Sprintf("%s, %d of %d issues closed", m.cfg.Milestone, len(closed), len(m.facts.Issues))
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, "; ") + ")"
	}
	return line
}

// formatComments turns control issue comments into the context block's
// lines: author and time, then the body.
func formatComments(comments []gh.Comment) []string {
	var out []string
	for _, c := range comments {
		out = append(out, fmt.Sprintf("%s at %s:\n%s", c.Author, c.CreatedAt.Local().Format("2006-01-02 15:04"), strings.TrimSpace(c.Body)))
	}
	return out
}

// pmQuestionMessage is what Huddle sends the companion when the PM set
// state:human-required: the PM's latest comment, and what to do with it.
func pmQuestionMessage(comments []gh.Comment, control int) string {
	question := "(Huddle could not read it; look at the control issue.)"
	if len(comments) > 0 {
		c := comments[len(comments)-1]
		question = fmt.Sprintf("by %s:\n%s", c.Author, strings.TrimSpace(c.Body))
	}
	return fmt.Sprintf("(Sent by Huddle, not typed by the human.) The PM has set %s and waits for the human. "+
		"Its latest comment on #%d, %s\n\nTell the human briefly what the PM needs and the options, so they can decide.",
		gh.StateHumanRequired, control, question)
}

// lastPMOutput returns the PM's final message from its newest run in runs
// (oldest first), or "" if there is none. A Codex PM wrote it to the
// .last file next to its log; a Claude Code PM's is in its JSON envelope,
// the last output line of its log.
func lastPMOutput(runs []logRun) string {
	for i := len(runs) - 1; i >= 0; i-- {
		if !strings.HasSuffix(runs[i].path, "-pm.log") {
			continue
		}
		last := strings.TrimSuffix(runs[i].path, ".log") + ".last"
		if data, err := os.ReadFile(last); err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
		lines := strings.Split(runner.StripStamps(readTail(runs[i].path, maxPMLogTail)), "\n")
		for j := len(lines) - 1; j >= 0; j-- {
			if !strings.HasPrefix(lines[j], "{") {
				continue
			}
			if env, err := agents.ParseClaude(lines[j]); err == nil && env.Result != "" {
				return env.Result
			}
		}
		return ""
	}
	return ""
}

// logTail returns the last n output lines of the log at path, without
// time stamps or control characters.
func logTail(path string, n int) string {
	lines := strings.Split(strings.TrimRight(readTail(path, maxLogWindow), "\n"), "\n")
	lines = lines[max(len(lines)-n, 0):]
	for i, l := range lines {
		_, text := runner.SplitStamp(l)
		lines[i] = cleanLogText(text)
	}
	return strings.Join(lines, "\n")
}

// readTail returns up to the last n bytes of the file at path, starting at
// a whole line, or "" if it cannot be read.
func readTail(path string, n int64) string {
	// A missing or unreadable log just leaves its part of the context
	// empty, so errors give "".
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	start := max(info.Size()-n, 0)
	data, err := io.ReadAll(io.NewSectionReader(f, start, info.Size()-start))
	if err != nil {
		return ""
	}
	if start > 0 {
		// Skip the cut-off first line.
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return string(data)
}

// decisionsText is the Decisions row of /status: how many decisions were
// agreed on this control issue, and the last one.
func (m Model) decisionsText() string {
	n, last := 0, companion.Entry{}
	for _, e := range m.chat {
		if e.Who == companion.WhoDecision {
			n, last = n+1, e
		}
	}
	if n == 0 {
		return "none yet"
	}
	text := []rune(last.Text)
	if len(text) > 60 {
		text = append([]rune(strings.TrimSpace(string(text[:57]))), '…')
	}
	return fmt.Sprintf("%d agreed · last %s %s", n, last.Time.In(m.now.Location()).Format("15:04"), string(text))
}
