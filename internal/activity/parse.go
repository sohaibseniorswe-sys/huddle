package activity

import (
	"regexp"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
)

// maxText is the longest event text a parser produces, in runes. Longer
// texts are cut and end in "…"; the raw log has the rest.
const maxText = 200

// maxCommand is the longest command shown in a "$ <command>" event.
const maxCommand = 80

// Codex turns the output of `codex exec` into events while it runs.
//
// codex prints its work in sections, each started by a header line on its
// own: "codex" (a message from the agent), "exec" (a shell command),
// "apply patch", "thinking", "user", "tokens used" and others. Codex reads
// those sections:
//
//   - each paragraph of a "codex" section becomes one event: its first
//     sentence;
//   - each "exec" section's command becomes "$ <command>   ok" (or with the
//     exit code) once the command finishes;
//   - everything else, including the "diff --git" blocks codex repeats after
//     every patch, is skipped.
//
// Feed it every line with Line, and call Flush when the run ends.
type Codex struct {
	logPath string
	emit    func(Event)

	section   string    // the current section: "codex", "exec", "diff" or "" (skipped)
	paragraph []string  // lines of the "codex" paragraph being read
	paraStart lineStart // where that paragraph started
	pending   []command // commands started but not finished, oldest first
}

// lineStart is where and when a line arrived.
type lineStart struct {
	time time.Time
	ref  LogRef
}

// command is a shell command codex started.
type command struct {
	text  string
	start lineStart
}

// NewCodex returns a Codex parser for output logged to logPath. It calls
// emit for every event it finds.
func NewCodex(logPath string, emit func(Event)) *Codex {
	return &Codex{logPath: logPath, emit: emit}
}

// codexHeaders are the lines that start a new section of codex output.
var codexHeaders = map[string]bool{
	"codex": true, "exec": true, "user": true, "thinking": true,
	"apply patch": true, "file update": true, "tokens used": true,
}

// codexDoneRe matches the line codex prints when a command finishes, for
// example " succeeded in 2804ms:" or " exited 1 in 275ms:". Commands may
// run in parallel, so this line can come long after its "exec" header.
var codexDoneRe = regexp.MustCompile(`^ (?:succeeded|exited (-?\d+)) in \d+ms:$`)

// Line reads one line of output.
func (c *Codex) Line(l runner.Line) {
	at := lineStart{time: l.Time, ref: LogRef{Path: c.logPath, Offset: l.Offset}}
	text := l.Text

	// A finished command can turn up in the middle of any section. The
	// command's output follows it and is skipped.
	if m := codexDoneRe.FindStringSubmatch(text); m != nil {
		c.endParagraph()
		c.finishCommand(m[1], l.Time)
		c.section = ""
		return
	}
	// Huddle's own lines at the start and end of the log end any section.
	if strings.HasPrefix(text, "# huddle: ") {
		c.endParagraph()
		c.section = ""
		return
	}
	if codexHeaders[text] {
		c.endParagraph()
		c.section = text
		if text != "codex" && text != "exec" {
			c.section = "" // a section Huddle does not show
		}
		return
	}
	if strings.HasPrefix(text, "diff --git ") {
		c.endParagraph()
		c.section = "diff" // skipped up to the next header
		return
	}

	switch c.section {
	case "exec":
		// The line after "exec" is the command. Its output follows; it is
		// skipped by leaving the section.
		c.pending = append(c.pending, command{text: commandText(text), start: at})
		c.section = ""
	case "codex":
		if strings.TrimSpace(text) == "" {
			c.endParagraph()
			return
		}
		if len(c.paragraph) == 0 {
			c.paraStart = at
		}
		c.paragraph = append(c.paragraph, strings.TrimSpace(text))
	}
}

// Flush reports what is still open when the run ends: a last paragraph, and
// commands that never finished.
func (c *Codex) Flush() {
	c.endParagraph()
	for _, cmd := range c.pending {
		c.emit(Event{Time: cmd.start.time, Actor: ActorCodex, Text: "$ " + cmd.text + "   (no result)", LogRef: cmd.start.ref})
	}
	c.pending = nil
}

// endParagraph turns the paragraph read so far into an event.
func (c *Codex) endParagraph() {
	if len(c.paragraph) == 0 {
		return
	}
	text := FirstSentence(strings.Join(c.paragraph, " "))
	c.paragraph = nil
	c.emit(Event{Time: c.paraStart.time, Actor: ActorCodex, Text: text, LogRef: c.paraStart.ref})
}

// finishCommand reports the oldest unfinished command. exitCode is "" for
// success. The event links to the command's line in the log, where its
// output follows.
func (c *Codex) finishCommand(exitCode string, at time.Time) {
	if len(c.pending) == 0 {
		return
	}
	cmd := c.pending[0]
	c.pending = c.pending[1:]
	result := "ok"
	if exitCode != "" {
		result = "exit " + exitCode
	}
	c.emit(Event{Time: at, Actor: ActorCodex, Text: "$ " + cmd.text + "   " + result, LogRef: cmd.start.ref})
}

// commandText shortens the command line codex prints after "exec", for
// example `/bin/bash -lc 'go test ./...' in /home/u/projects/palantir`, to
// the command itself: `go test ./...`.
func commandText(line string) string {
	s := strings.TrimSpace(line)
	if i := strings.LastIndex(s, " in /"); i >= 0 {
		s = s[:i]
	}
	for _, shell := range []string{"/bin/bash -lc ", "/bin/zsh -lc ", "/bin/sh -c ", "bash -lc "} {
		s = strings.TrimPrefix(s, shell)
	}
	// Remove the shell quotes around the whole command.
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return shorten(s, maxCommand)
}

// Claude turns the output of `claude -p --output-format json` into events.
// Claude Code prints nothing while it works; at the end it prints one JSON
// envelope, and the first sentence of its result becomes the event.
type Claude struct {
	logPath string
	emit    func(Event)
}

// NewClaude returns a Claude parser for output logged to logPath. It calls
// emit for every event it finds.
func NewClaude(logPath string, emit func(Event)) *Claude {
	return &Claude{logPath: logPath, emit: emit}
}

// Line reads one line of output.
func (c *Claude) Line(l runner.Line) {
	if !strings.HasPrefix(l.Text, "{") {
		return
	}
	env, err := agents.ParseClaude(l.Text)
	if err != nil || strings.TrimSpace(env.Result) == "" {
		return // not the envelope; the raw log still has the line
	}
	text := FirstSentence(env.Result)
	if env.IsError {
		text = "Error: " + text
	}
	c.emit(Event{Time: l.Time, Actor: ActorClaude, Text: text, LogRef: LogRef{Path: c.logPath, Offset: l.Offset}})
}

// Flush does nothing: Claude Code's events are complete when their line
// arrives. It exists so both parsers are used the same way.
func (c *Claude) Flush() {}

// sentenceEndRe matches the end of a sentence: '.', '!' or '?' followed by
// white space.
var sentenceEndRe = regexp.MustCompile(`[.!?]\s`)

// FirstSentence returns the first sentence of s on one line, without the
// final full stop, cut to a short length.
func FirstSentence(s string) string {
	s = strings.Join(strings.Fields(s), " ") // one line, single spaces
	if loc := sentenceEndRe.FindStringIndex(s + " "); loc != nil {
		s = s[:loc[0]+1]
	}
	s = strings.TrimSuffix(s, ".")
	return shorten(s, maxText)
}

// shorten cuts s to at most n runes, ending in "…" if it was cut.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}
