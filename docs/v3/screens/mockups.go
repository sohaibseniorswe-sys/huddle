//go:build ignore

// Command mockups renders Huddle v3 screens with Lip Gloss, exactly as a
// Bubble Tea View() would return them, and prints them with ANSI colours.
//
// Layout (top to bottom), H rows in total:
//
//	project pane  2 rows, fixed
//	activity      flexible: all remaining space by default
//	companion     when open: takes half of the activity's space
//	menu / card   when open: takes space from the companion (or activity)
//	text field    3 rows, fixed
//	help line     1 row, fixed
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

const W = 100
const H = 34

var (
	purple = lipgloss.Color("#7D56F4")
	green  = lipgloss.Color("#3FB950")
	amber  = lipgloss.Color("#E3A008")
	blue   = lipgloss.Color("#58A6FF")
	pink   = lipgloss.Color("#D2A8FF")
	text   = lipgloss.Color("#E6E6EF")
	dim    = lipgloss.Color("#8B8B9A")
	faint  = lipgloss.Color("#4A4A5A")
	barBg  = lipgloss.Color("#2A2A3A")
	white  = lipgloss.Color("#FFFFFF")
	black  = lipgloss.Color("#111111")
)

func fg(c lipgloss.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

var (
	bold     = lipgloss.NewStyle().Bold(true).Foreground(text)
	plain    = fg(text)
	muted    = fg(dim)
	keyStyle = lipgloss.NewStyle().Foreground(text).Bold(true)
)

// ---------------------------------------------------------------- pieces

func projectPane(state string, stateColor lipgloss.Color, issues string) string {
	badge := lipgloss.NewStyle().Bold(true).Foreground(white).Background(purple).Padding(0, 1).Render("HUDDLE")
	info := lipgloss.NewStyle().Foreground(text).Background(barBg).Padding(0, 1).
		Render("palantir · Milestone 3 · control #58 · autonomy 8")
	right := lipgloss.NewStyle().Bold(true).Foreground(black).Background(stateColor).Padding(0, 1).Render("● " + state)
	gap := W - lipgloss.Width(badge) - lipgloss.Width(info) - lipgloss.Width(right)
	bar := badge + info + lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", gap)) + right
	parts := []string{
		muted.Render("Issues ") + plain.Render(issues),
		muted.Render("PRs ") + plain.Render("#73 open"),
		muted.Render("CI ") + fg(green).Render("green"),
		muted.Render("Usage ") + plain.Render("Codex ok · Claude ok"),
		muted.Render("Elapsed ") + plain.Render("2h14m"),
	}
	return bar + "\n " + strings.Join(parts, muted.Render("  ·  "))
}

// pane draws a bordered pane of exactly h rows. Long content keeps its last
// lines (tail=true, for streams) or its first lines (tail=false, for views).
func pane(title, hint string, body string, h int, focused bool, accent lipgloss.Color, tail bool) string {
	border := faint
	if focused {
		border = purple
	}
	inner := W - 4
	wrapped := strings.Split(lipgloss.NewStyle().Width(inner).Render(body), "\n")
	room := h - 3
	if len(wrapped) > room {
		if tail {
			wrapped = wrapped[len(wrapped)-room:]
		} else {
			wrapped = wrapped[:room]
		}
	}
	for len(wrapped) < room {
		wrapped = append(wrapped, "")
	}
	t := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(title)
	if focused {
		t += fg(purple).Render("  ◆ focused")
	}
	if hint != "" {
		gap := inner - lipgloss.Width(t) - lipgloss.Width(hint)
		t += strings.Repeat(" ", max(gap, 1)) + muted.Render(hint)
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(0, 1).Width(W - 2).Render(t + "\n" + strings.Join(wrapped, "\n"))
}

func input(prefix string, prefixColor lipgloss.Color, value string, placeholder bool) string {
	v := plain.Render(value)
	if placeholder {
		v = muted.Render(value)
	}
	cursor := lipgloss.NewStyle().Background(text).Render(" ")
	if placeholder {
		v = cursor + v
		cursor = ""
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple).
		Padding(0, 1).Width(W - 2).Render(fg(prefixColor).Bold(true).Render(prefix+" › ") + v + cursor)
}

func help(items ...string) string {
	var out []string
	for i := 0; i+1 < len(items); i += 2 {
		out = append(out, keyStyle.Render(items[i])+" "+muted.Render(items[i+1]))
	}
	return " " + strings.Join(out, muted.Render("  ·  "))
}

type ev struct{ t, who, msg string }

func actor(who string) string {
	c := map[string]lipgloss.Color{"codex": blue, "pm": pink, "huddle": dim, "you": green}[who]
	return lipgloss.NewStyle().Foreground(c).Bold(true).Width(7).Render(who)
}

func activity(events []ev, selected int) string {
	var lines []string
	for i, e := range events {
		if i == selected {
			row := "▸ " + e.t + "  " + fmt.Sprintf("%-7s", e.who) + " " + e.msg + "   (enter: open raw log here)"
			lines = append(lines, lipgloss.NewStyle().Background(lipgloss.Color("#3A3456")).Foreground(white).Bold(true).Width(W-4).Render(row))
			continue
		}
		head := "  " + muted.Render(e.t+"  ") + actor(e.who) + " "
		body := lipgloss.NewStyle().Foreground(text).Width(W - 4 - lipgloss.Width(head)).Render(e.msg)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, head, body))
	}
	return strings.Join(lines, "\n")
}

func chat(lines []ev) string {
	var out []string
	for _, l := range lines {
		if l.who == "you" {
			out = append(out, fg(green).Bold(true).Render("you  ")+plain.Render(l.msg))
		} else {
			out = append(out, fg(amber).Bold(true).Render("companion  ")+plain.Render(l.msg))
		}
	}
	return strings.Join(out, "\n\n")
}

type cmd struct{ name, desc string }

func menu(cmds []cmd, h int) string {
	room := h - 2
	more := 0
	if len(cmds) > room {
		more = len(cmds) - (room - 1)
		cmds = cmds[:room-1]
	}
	var rows []string
	for i, c := range cmds {
		lit := strings.HasSuffix(c.name, " ●")
		base := strings.TrimSuffix(c.name, " ●")
		name := fmt.Sprintf("%-15s", base)
		if i == 0 {
			rows = append(rows, lipgloss.NewStyle().Background(purple).Foreground(white).Bold(true).Width(W-4).Render(" "+name+c.desc))
		} else if lit {
			rows = append(rows, " "+keyStyle.Render(base)+fg(amber).Bold(true).Render(" ●")+strings.Repeat(" ", 15-len(base)-2)+muted.Render(c.desc))
		} else {
			rows = append(rows, " "+keyStyle.Render(name)+muted.Render(c.desc))
		}
	}
	if more > 0 {
		rows = append(rows, muted.Render(fmt.Sprintf(" ↓ %d more", more)))
	}
	for len(rows) < room {
		rows = append(rows, "")
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple).Width(W - 2).Render(strings.Join(rows, "\n"))
}

func screen(parts ...string) string {
	return lipgloss.NewStyle().Width(W).Height(H).MaxHeight(H).Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// ---------------------------------------------------------------- content

var events = []ev{
	{"20:39", "huddle", "Started Milestone 3 · control #58 · autonomy 8"},
	{"20:40", "codex", "Read #58, AGENTS.md and the 7 milestone issues"},
	{"20:58", "codex", "Merged PR #68, closed #59"},
	{"21:31", "codex", "Merged PR #69, closed #60"},
	{"22:06", "codex", "Merged PR #70, closed #63"},
	{"22:41", "codex", "Merged PR #71, closed #61"},
	{"22:41", "codex", "Starting #62: add the /export endpoint"},
	{"22:43", "codex", "$ go test ./...   ok"},
	{"22:47", "codex", "Added export handler and table-driven tests"},
	{"22:52", "codex", "$ gh pr create   → PR #73"},
	{"22:52", "codex", "Waiting for CI on PR #73"},
	{"22:53", "huddle", "Usage check: Codex ok, Claude Code ok"},
}

var talk = []ev{
	{"", "you", "Why JSON and not CSV for the export?"},
	{"", "companion", "The PM decided that at 23:03, after Codex asked in its handover on #58. The dashboard in #64 reads JSON, so CSV would need a second parser."},
	{"", "you", "Could we add CSV later?"},
	{"", "companion", "Yes, as a small follow-up issue. Codex is on #62 now and CI for PR #73 is running. Want me to draft a decision?"},
}

const flex = H - 2 - 3 - 1 // rows between the project pane and the text field

// split returns the heights of Activity, Companion and the slot (menu or
// card). Activity alone takes everything; Companion takes half; with all
// three open: 1/4 Activity, 1/2 Companion, 1/4 slot.
func split(companion, slot bool) (a, c, m int) {
	switch {
	case companion && slot:
		a, m = flex/4, flex/4
		c = flex - a - m
	case companion:
		a = flex / 2
		c = flex - a
	case slot:
		m = flex / 4
		a = flex - m
	default:
		a = flex
	}
	return
}

var allCmds = []cmd{
	{"/status", "facts: issues, PRs, CI, turns, usage, elapsed"},
	{"/companion ●", "open or close the companion pane (open now)"},
	{"/decide", "draft a decision from the companion conversation"},
	{"/pause", "let the current turn finish, start no new one"},
	{"/log", "raw output of every agent run, scrollable"},
	{"/settings", "this project's settings, read-only"},
	{"/quit", "stop the running agent now; it resumes on next start"},
}

// ---------------------------------------------------------------- screens

func sDefault() string {
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Activity", "", activity(events, -1), flex, true, dim, true),
		input("companion", amber, "Type to talk to the companion, / for commands", true),
		help("enter", "send", "/", "commands", "pgup/pgdn", "scroll", "ctrl+c", "quit"))
}

func sCompanion() string {
	a, c, _ := split(true, false)
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Activity", "", activity(events, -1), a, false, dim, true),
		pane("Companion", "/companion to close", chat(talk), c, true, amber, true),
		input("companion", amber, "Not now. Put CSV in Milestone 4.", false),
		help("enter", "send", "tab", "switch pane", "pgup/pgdn", "scroll focused pane", "/", "commands"))
}

func sMenu() string {
	a, c, m := split(true, true)
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Activity", "", activity(events, -1), a, false, dim, true),
		pane("Companion", "", chat(talk), c, false, amber, true),
		menu(allCmds, m),
		input("command", purple, "/", false),
		help("↑/↓", "choose", "tab", "complete", "enter", "run", "esc", "close menu"))
}

func sFilter() string {
	a, c, m := split(true, true)
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Activity", "", activity(events, -1), a, false, dim, true),
		pane("Companion", "", chat(talk), c, false, amber, true),
		menu([]cmd{allCmds[0], allCmds[5]}, m),
		input("command", purple, "/s", false),
		help("↑/↓", "choose", "tab", "complete", "enter", "run", "esc", "close menu"))
}

func sPaused() string {
	evs := append(append([]ev{}, events[5:]...),
		ev{"23:10", "you", "/pause"},
		ev{"23:10", "huddle", "Pausing: Codex's current turn will finish, then nothing new starts"},
		ev{"23:14", "codex", "Handed over to PM (Progress): #62 merged, #64 next"},
		ev{"23:14", "huddle", "Paused. PM's turn is next and starts when you /resume"},
	)
	cmds := []cmd{
		{"/status", "facts: issues, PRs, CI, turns, usage, elapsed"},
		{"/companion", "open or close the companion pane"},
		{"/resume", "start the PM's turn now"},
		{"/log", "raw output of every agent run, scrollable"},
		{"/settings", "this project's settings, read-only"},
		{"/quit", "quit Huddle; the milestone resumes on next start"},
	}
	a, _, m := split(false, true)
	return screen(projectPane("Paused · PM next", amber, "5/7 closed"),
		pane("Activity", "", activity(evs, -1), a, false, dim, true),
		menu(cmds, m),
		input("command", purple, "/", false),
		help("↑/↓", "choose", "enter", "run", "esc", "close menu"))
}

func sStatus() string {
	kv := func(k, v string) string { return muted.Width(14).Render(k) + v }
	bar := fg(green).Render("■■■■") + fg(faint).Render("■■■")
	body := "\n" + strings.Join([]string{
		kv("Turn", plain.Render("Dev (Codex) · working 12m · turn 5")),
		kv("Issues", bar+plain.Render("  4 of 7 closed  (#59 #60 #61 #63)")),
		kv("Open PRs", plain.Render("#73 export endpoint · CI running (1 of 2 checks)")),
		kv("Main", fg(green).Render("green")+plain.Render(" · last merge 22:41, PR #71")),
		kv("Decisions", plain.Render("1 today · 23:03 export format JSON (PM)")),
		kv("Autonomy", plain.Render("8 · Autonomous")),
		kv("Usage", plain.Render("Codex ok · Claude Code ok · no waits")),
		kv("Elapsed", plain.Render("2h14m · started 20:39")),
		"",
		muted.Render("Answered by Huddle from GitHub and its logs. No agent was started."),
	}, "\n")
	a, c, _ := split(true, false)
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Status", "esc: back to activity", strings.TrimPrefix(body, "\n"), a, true, purple, false),
		pane("Companion", "", chat(talk), c, false, amber, true),
		input("companion", amber, "Type to talk to the companion, / for commands", true),
		help("esc", "back", "/", "commands", "tab", "switch pane"))
}

func sLog() string {
	ts := func(t string) string { return muted.Render(t + "  ") }
	raw := fg(lipgloss.Color("#C9C9D6"))
	lines := []string{
		ts("22:51:58") + raw.Render("exec"),
		ts("22:51:58") + raw.Render(`/bin/bash -lc "go vet ./... && go test ./..." in ~/projects/palantir`),
		ts("22:52:01") + raw.Render(" succeeded in 2804ms:"),
		ts("22:52:01") + raw.Render("ok      github.com/sohaibseniorswe-sys/palantir/internal/export  0.011s"),
		lipgloss.NewStyle().Background(lipgloss.Color("#3A3456")).Foreground(white).Bold(true).Width(W - 4).Render("22:52:07  exec      ← you jumped here from Activity"),
		ts("22:52:07") + raw.Render(`/bin/bash -lc "git push -u origin agent/issue-62 && gh pr create --title 'Add /export'`),
		ts("22:52:07") + raw.Render(`  --body 'Closes #62'" in ~/projects/palantir`),
		ts("22:52:11") + raw.Render(" succeeded in 4231ms:"),
		ts("22:52:11") + raw.Render(" * [new branch]      agent/issue-62 -> agent/issue-62"),
		ts("22:52:11") + raw.Render("https://github.com/sohaibseniorswe-sys/palantir/pull/73"),
		ts("22:52:12") + raw.Render(""),
		ts("22:52:12") + raw.Render("codex"),
		ts("22:52:12") + raw.Render("PR #73 is open. Waiting for CI before merging and moving on to #64."),
		ts("22:52:13") + raw.Render(""),
		ts("22:52:13") + raw.Render("exec"),
		ts("22:52:13") + raw.Render(`/bin/bash -lc 'gh pr checks 73 --watch --interval 10' in ~/projects/palantir`),
		ts("22:52:23") + raw.Render("test    pending 0   https://github.com/sohaibseniorswe-sys/palantir/actions/runs/38461"),
		ts("22:52:33") + raw.Render("test    pending 0   https://github.com/sohaibseniorswe-sys/palantir/actions/runs/38461"),
		ts("22:52:43") + raw.Render("test    pass    31s https://github.com/sohaibseniorswe-sys/palantir/actions/runs/38461"),
	}
	a, c, _ := split(true, false)
	conv := []ev{
		{"", "you", "What am I looking at?"},
		{"", "companion", "Codex's raw log for turn 5 at 22:52:07. It pushed agent/issue-62, opened PR #73 for the /export endpoint and started watching CI, which passed 36 seconds later at 22:52:43."},
	}
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Raw log · Codex · turn 5 (12 runs)", "[ ] prev/next run · esc: back", strings.Join(lines, "\n"), a, true, purple, false),
		pane("Companion", "", chat(conv), c, false, amber, true),
		input("companion", amber, "Type to talk to the companion, / for commands", true),
		help("pgup/pgdn", "scroll", "[ ]", "prev/next run", "g/G", "start/end", "esc", "back"))
}

func sSelect() string {
	a, c, _ := split(true, false)
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Activity", "↑/↓ select · enter open", activity(events, 9), a, true, dim, true),
		pane("Companion", "", chat(talk), c, false, amber, true),
		input("companion", amber, "Type to talk to the companion, / for commands", true),
		help("↑/↓", "select line", "enter", "open raw log there", "tab", "switch pane", "esc", "stop selecting"))
}

func sSettings() string {
	sec := func(s string) string { return fg(purple).Bold(true).Render(s) }
	kv := func(k, v string) string { return "  " + muted.Width(16).Render(k) + plain.Render(v) }
	body := strings.Join([]string{
		muted.Render("~/.huddle/projects/sohaibseniorswe-sys_palantir.yaml"),
		"",
		sec("Project"),
		kv("Repo", "sohaibseniorswe-sys/palantir"),
		kv("Folder", "~/projects/palantir"),
		kv("Control issue", "#58 · Milestone 3"),
		"",
		sec("Agents"),
		kv("Dev", "Codex 0.160.0 · signed in with ChatGPT"),
		kv("PM", "Claude Code 2.1.296 · signed in with claude.ai"),
		kv("Companion", "Claude Code (follows the PM)"),
		"",
		sec("Autonomy"),
		kv("Level", "8 · Autonomous"),
		"",
		sec("Files"),
		kv("Logs", "~/.huddle/logs/  (12 runs this milestone)"),
		kv("Sessions", "Codex 01a1…f981 · PM 717c…f5"),
		"",
		muted.Render("Read-only. To change settings, quit and run: huddle setup"),
	}, "\n")
	return screen(projectPane("Dev (Codex) working", blue, "4/7 closed"),
		pane("Settings", "esc: back to activity", body, flex, true, purple, false),
		input("companion", amber, "Type to talk to the companion, / for commands", true),
		help("esc", "back", "/", "commands", "ctrl+c", "quit"))
}

func decisionScreen(cardFocused bool) string {
	evs := []ev{
		{"23:41", "codex", "Merged PR #74, closed #64"},
		{"23:58", "codex", "Closed #66; #65 blocked: no MAPBOX_TOKEN"},
		{"23:59", "codex", "Handed over to PM (Decision needed)"},
		{"00:02", "pm", "Needs you: #65 needs a Mapbox token"},
		{"00:02", "huddle", "Waiting for you. No agent is running."},
	}
	conv := []ev{
		{"", "companion", "The PM needs a token only you can create. Options: Mapbox (token), MapLibre with free OpenStreetMap tiles (no key), or skip the map for now."},
		{"", "you", "Use OpenStreetMap for now. I'll get a Mapbox token next week."},
		{"", "companion", "MapLibre with OSM tiles needs no key and keeps #65's acceptance criteria. Here's the decision:"},
	}
	var agree, keep, title, hint string
	border := lipgloss.RoundedBorder()
	if cardFocused {
		agree = lipgloss.NewStyle().Bold(true).Foreground(black).Background(green).Padding(0, 2).Render("▸ Agree")
		keep = lipgloss.NewStyle().Foreground(text).Background(barBg).Padding(0, 2).Render("Keep talking")
		title = fg(green).Bold(true).Render("DECISION") + fg(purple).Render("  ◆ focused")
		hint = muted.Render("←/→ choose · enter confirm · esc back to typing")
		border = lipgloss.ThickBorder()
	} else {
		agree = muted.Background(barBg).Padding(0, 2).Render("Agree")
		keep = muted.Background(barBg).Padding(0, 2).Render("Keep talking")
		title = fg(green).Bold(true).Render("DECISION")
		hint = keyStyle.Render("/decision") + muted.Render(" to answer")
	}
	gap := W - 4 - lipgloss.Width(title) - lipgloss.Width(hint)
	card := lipgloss.NewStyle().Border(border).BorderForeground(green).Padding(0, 1).Width(W - 2).Height(4).Render(
		title + strings.Repeat(" ", gap) + hint + "\n" +
			lipgloss.NewStyle().Foreground(text).Width(W-4).Render("Use MapLibre with OpenStreetMap tiles for #65 now (no API key). Add a follow-up issue in Milestone 4 to switch to Mapbox once the token exists.") + "\n" +
			agree + "  " + keep)
	a, c, m := split(true, true)
	card = lipgloss.NewStyle().Height(m).Render(card)
	in := input("companion", amber, "Is MapLibre as fast as Mapbox on mobile?", false)
	hl := help("enter", "send", "/decision", "answer the decision", "tab", "switch pane", "/", "commands")
	if cardFocused {
		in = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(faint).Padding(0, 1).Width(W - 2).Render(
			muted.Bold(true).Render("companion › ") + muted.Render("Is MapLibre as fast as Mapbox on mobile?   (kept while you answer)"))
		hl = help("←/→", "choose", "enter", "confirm", "esc", "back to typing")
	}
	return screen(projectPane("Waiting for you", amber, "6/7 closed"),
		pane("Activity", "", activity(evs, -1), a, false, dim, true),
		pane("Companion", "", chat(conv), c, false, amber, true),
		card, in, hl)
}

func sDecision() string        { return decisionScreen(false) }
func sDecisionFocused() string { return decisionScreen(true) }

func wizardFrame(step int, title, detail string) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(white).Background(purple).Padding(0, 1).Render("HUDDLE SETUP")
	where := lipgloss.NewStyle().Foreground(text).Background(barBg).Padding(0, 1).Render("~/projects/palantir")
	stepTxt := lipgloss.NewStyle().Foreground(text).Background(barBg).Padding(0, 1).Render(fmt.Sprintf("step %d of 9", step))
	gap := W - lipgloss.Width(head) - lipgloss.Width(where) - lipgloss.Width(stepTxt)
	bar := head + where + lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", gap)) + stepTxt
	steps := []struct{ name, val string }{
		{"Project folder", "~/projects/palantir"},
		{"GitHub repo", "sohaibseniorswe-sys/palantir   (gh: sohaibseniorswe-sys)"},
		{"Dev agent", "Codex · gpt-6.1-sol · effort medium"},
		{"PM agent", "Claude Code · Opus 5.5 · effort high"},
		{"Companion", "Claude Code · Sonnet 5.5 · effort medium"},
		{"Logins", "Codex: ChatGPT · Claude Code: claude.ai · no API keys"},
		{"Autonomy level", ""},
		{"Repo preparation", ""},
		{"Save", ""},
	}
	var rows []string
	for i, s := range steps {
		n := i + 1
		name := fmt.Sprintf("%d  %-18s", n, s.name)
		switch {
		case n < step:
			rows = append(rows, fg(green).Render("  ✓ ")+plain.Render(name)+muted.Render(s.val))
		case n == step:
			rows = append(rows, fg(purple).Bold(true).Render("  ▸ ")+bold.Render(name))
		default:
			rows = append(rows, muted.Render("    "+name))
		}
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple).
		Padding(1, 2).Width(70).Render(bold.Render(title) + "\n\n" + detail)
	return lipgloss.NewStyle().Width(W).Height(H - 1).Render(
		lipgloss.JoinVertical(lipgloss.Left, bar, "", strings.Join(rows, "\n"), "", lipgloss.NewStyle().MarginLeft(2).Render(card)))
}

func chips(opts []string, sel int, active bool) string {
	var out []string
	for i, o := range opts {
		switch {
		case i == sel && active:
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(white).Background(purple).Padding(0, 1).Render(o))
		case i == sel:
			out = append(out, lipgloss.NewStyle().Bold(true).Foreground(text).Background(barBg).Padding(0, 1).Render(o))
		default:
			out = append(out, muted.Padding(0, 1).Render(o))
		}
	}
	return strings.Join(out, " ")
}

func field(label string, focused bool, value string) string {
	mark := "  "
	l := muted
	if focused {
		mark, l = fg(purple).Bold(true).Render("▸ "), bold
	}
	return mark + l.Width(9).Render(label) + value
}

func wizardAgent() string {
	detail := field("CLI", false, chips([]string{"Claude Code", "Codex"}, 0, false)) + "\n\n" +
		field("Model", true, chips([]string{"Opus 5.5", "Sonnet 5.5", "Haiku 5.5", "Custom…"}, 0, true)) + "\n\n" +
		field("Effort", false, chips([]string{"low", "medium", "high", "xhigh", "max"}, 2, false)) + "\n\n" +
		muted.Render("Known models for Claude Code, or Custom… to type any model\nname. Step 6 checks every choice with a short test run.")
	return wizardFrame(4, "PM agent", detail) + "\n" + help("↑/↓", "field", "←/→", "choose", "enter", "confirm", "esc", "back")
}

func wizardCompanion() string {
	detail := field("Source", false, chips([]string{"Same as PM", "Choose separately"}, 1, false)) + "\n\n" +
		field("CLI", false, chips([]string{"Claude Code", "Codex"}, 0, false)) + "\n\n" +
		field("Model", false, chips([]string{"Opus 5.5", "Sonnet 5.5", "Haiku 5.5", "Custom…"}, 1, false)) + "\n\n" +
		field("Effort", true, chips([]string{"low", "medium", "high", "xhigh", "max"}, 1, true)) + "\n\n" +
		muted.Render("The companion only talks and reads, so a lighter model\nanswers faster and saves the PM's limits.")
	return wizardFrame(5, "Companion", detail) + "\n" + help("↑/↓", "field", "←/→", "choose", "enter", "confirm", "esc", "back")
}

func wizardAutonomy() string {
	level := 8
	var track strings.Builder
	for i := 0; i <= 10; i++ {
		switch {
		case i == level:
			track.WriteString(fg(purple).Bold(true).Render("●"))
			if i < 10 {
				track.WriteString(fg(faint).Render("━━━"))
			}
		case i < level:
			track.WriteString(fg(purple).Render("━━━━"))
		default:
			track.WriteString(fg(faint).Render("━━━━"))
		}
	}
	scale := muted.Render("0") + " " + track.String() + " " + muted.Render("10")
	row := func(k, v string) string {
		return lipgloss.JoinHorizontal(lipgloss.Top, muted.Width(21).Render(k), plain.Width(43).Render(v))
	}
	detail := scale + "\n\n" + bold.Render("8 · Autonomous") + "\n\n" +
		row("Dev goes to the PM", "only for conflicts between issues, requirement changes or real blockers") + "\n\n" +
		row("PM comes to you", "only when the world is on fire: credentials, money, access, or no way forward")
	return wizardFrame(7, "How independently should the agents work?", detail) + "\n" + help("←/→", "change", "enter", "confirm", "esc", "back")
}

func startFrame(title string, opts []string, notes []string, sel int, foot string) string {
	head := lipgloss.NewStyle().Bold(true).Foreground(white).Background(purple).Padding(0, 1).Render("HUDDLE")
	where := lipgloss.NewStyle().Foreground(text).Background(barBg).Padding(0, 1).Render("palantir · ~/projects/palantir")
	gap := W - lipgloss.Width(head) - lipgloss.Width(where)
	bar := head + where + lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", gap))
	var rows []string
	for i, o := range opts {
		if i == sel {
			rows = append(rows, fg(purple).Bold(true).Render("> ")+bold.Width(42).Render(o)+muted.Render(notes[i]))
		} else {
			rows = append(rows, "  "+plain.Width(42).Render(o)+muted.Render(notes[i]))
		}
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(purple).
		Padding(1, 2).Width(W - 6).Render(bold.Render(title) + "\n\n" + strings.Join(rows, "\n\n") + "\n\n" + muted.Render(foot))
	return lipgloss.NewStyle().Width(W).Height(H - 1).Render(
		lipgloss.JoinVertical(lipgloss.Left, bar, "", lipgloss.NewStyle().MarginLeft(2).Render(card)))
}

func startMenu() string {
	return startFrame("What do you want to do?",
		[]string{"Plan new milestones with the companion", "Run an existing milestone", "Resume Milestone 3", "Settings", "Quit"},
		[]string{"talk, agree, Huddle creates them", "pick one of 2 open milestones", "control #58 · Dev working when stopped", "read-only", ""},
		0, "Planned milestones run one after another, each with its own control issue.") + "\n" +
		help("↑/↓", "choose", "enter", "open", "q", "quit")
}

func pickMilestone() string {
	return startFrame("Which milestone should Huddle run?",
		[]string{"Milestone 4 · Maps and exports", "Milestone 5 · Admin dashboard"},
		[]string{"6 issues · control issue created for you", "4 issues · 1 closed · control #81"},
		0, "Optional: type a first message for the Dev after choosing, or press enter\nto start with \"Implement this milestone.\"") + "\n" +
		help("↑/↓", "choose", "enter", "start", "esc", "back")
}

func main() {
	lipgloss.SetColorProfile(termenv.TrueColor)
	screens := map[string]func() string{
		"14-start-menu":        startMenu,
		"15-pick-milestone":    pickMilestone,
		"01-default":           sDefault,
		"02-companion":         sCompanion,
		"03-menu":              sMenu,
		"04-menu-filter":       sFilter,
		"05-paused":            sPaused,
		"06-status":            sStatus,
		"07-activity-select":   sSelect,
		"08-log":               sLog,
		"09-settings":          sSettings,
		"10-decision":          sDecision,
		"10b-decision-focused": sDecisionFocused,
		"11-wizard-pm":         wizardAgent,
		"12-wizard-companion":  wizardCompanion,
		"13-wizard-autonomy":   wizardAutonomy,
	}
	for name, f := range screens {
		out := f()
		if n := lipgloss.Height(out); n != H {
			fmt.Fprintf(os.Stderr, "%s: %d rows, want %d\n", name, n, H)
		}
		if err := os.WriteFile(name+".ans", []byte(out), 0o644); err != nil {
			panic(err)
		}
	}
}
