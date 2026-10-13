package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/ui"
)

// maxLogWindow is the most of one log file the /log view reads at a time.
// Logs of long turns grow to many megabytes; the view reads the part
// around the line it shows, and g / G read the start or the end.
const maxLogWindow = 1 << 20 // 1 MiB

// logContextRows is how many rows above the line jumped to stay visible.
const logContextRows = 4

// stampWidth is the width of a log line's time stamp, "22:52:07".
const stampWidth = len(runner.StampLayout)

// logRun is one agent run's log file.
type logRun struct {
	path  string
	agent string // "Codex" or "Claude Code"
	turn  int    // the turn number in the file name
}

// logLine is one line of a log file.
type logLine struct {
	stamp  string // "22:52:07", or "" in logs from before v3
	text   string
	offset int64 // where the line starts in the file
}

// logView is the state of the /log view: which run is shown and the part
// of its file that is loaded.
type logView struct {
	runs   []logRun  // this repo's runs, oldest first
	run    int       // the run shown, an index into runs
	lines  []logLine // the loaded part of the file
	from   int64     // the offset of the first loaded line
	to     int64     // the offset just after the last loaded line
	size   int64     // the file's size when it was read
	target int       // the line jumped to from Activity, an index into lines; -1 if none
	err    error     // the last read's error
	vp     viewport.Model
}

// path returns the shown run's log file, or "" if there is none.
func (v logView) path() string {
	if v.run < 0 || v.run >= len(v.runs) {
		return ""
	}
	return v.runs[v.run].path
}

// atStart and atEnd report whether the loaded part reaches the start and
// the end of the file.
func (v logView) atStart() bool { return v.from == 0 }
func (v logView) atEnd() bool   { return v.to >= v.size }

// openLatestLog opens the /log view at the end of the newest run.
func (m Model) openLatestLog() Model {
	m.log.runs, m.log.err = findRuns(m.cfg.LogDir, m.cfg.RepoDir)
	m.log.run = len(m.log.runs) - 1
	m.log = m.log.reloadEnd()
	m.view, m.focus = viewLog, focusActivity
	m = m.sync()
	m.log.vp.GotoBottom()
	return m
}

// openLog opens the /log view at the line ref points to, with a few lines
// of context above it.
func (m Model) openLog(ref activity.LogRef) Model {
	m.log.runs, m.log.err = findRuns(m.cfg.LogDir, m.cfg.RepoDir)
	m.log.run = -1
	for i, r := range m.log.runs {
		if r.path == ref.Path {
			m.log.run = i
		}
	}
	if m.log.run < 0 {
		// A run from another folder, or a log that is gone: show it anyway.
		m.log.runs = append(m.log.runs, logRun{path: ref.Path, agent: "agent"})
		m.log.run = len(m.log.runs) - 1
	}
	m.log = m.log.load(ref.Offset - maxLogWindow/2)
	m.log.target = -1
	for i, l := range m.log.lines {
		if l.offset <= ref.Offset {
			m.log.target = i
		}
	}
	m.view, m.focus = viewLog, focusActivity
	m = m.sync()
	m.log.vp.SetYOffset(max(m.log.targetRow()-logContextRows, 0))
	return m
}

// logKey handles a key in the /log view.
func (m Model) logKey(k tea.KeyMsg) Model {
	switch k.String() {
	case "[":
		if m.log.run > 0 {
			m.log.run--
			m.log = m.log.reloadEnd()
			m = m.sync()
			m.log.vp.GotoBottom()
		}
	case "]":
		if m.log.run < len(m.log.runs)-1 {
			m.log.run++
			m.log = m.log.reloadEnd()
			m = m.sync()
			m.log.vp.GotoBottom()
		}
	case "g":
		if !m.log.atStart() {
			m.log = m.log.load(0)
			m = m.sync()
		}
		m.log.vp.GotoTop()
	case "G":
		if !m.log.atEnd() {
			m.log = m.log.reloadEnd()
			m = m.sync()
		}
		m.log.vp.GotoBottom()
	default:
		scroll(&m.log.vp, k)
	}
	return m
}

// reloadEnd loads the end of the shown run's file.
func (v logView) reloadEnd() logView {
	v.target = -1
	return v.load(1 << 62) // past any end; load moves it back
}

// load reads up to maxLogWindow bytes of the shown run's file, starting at
// the first whole line at or after from.
func (v logView) load(from int64) logView {
	v.lines, v.from, v.to, v.size, v.err = nil, 0, 0, 0, nil
	if v.path() == "" {
		return v
	}
	v.lines, v.from, v.to, v.size, v.err = readWindow(v.path(), from)
	return v
}

// readWindow reads the whole lines in about maxLogWindow bytes of the file
// at path from offset from. from is moved back if less than a window is
// left after it. It returns the lines, the offsets where they start and
// end, and the file's size.
func readWindow(path string, from int64) (lines []logLine, start, end, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, 0, 0, 0, err
	}
	size = info.Size()
	start = min(max(from, 0), max(size-maxLogWindow, 0))
	buf := make([]byte, min(maxLogWindow, size-start))
	n, err := f.ReadAt(buf, start)
	if err != nil && err != io.EOF {
		return nil, 0, 0, 0, err
	}
	buf = buf[:n]

	// Skip a cut-off first line, and drop a cut-off last line.
	if start > 0 {
		i := bytes.IndexByte(buf, '\n')
		start += int64(i + 1)
		buf = buf[i+1:]
	}
	if start+int64(len(buf)) < size {
		buf = buf[:bytes.LastIndexByte(buf, '\n')+1]
	}

	offset := start
	for _, raw := range strings.SplitAfter(string(buf), "\n") {
		if raw == "" {
			continue
		}
		stamp, text := runner.SplitStamp(strings.TrimSuffix(raw, "\n"))
		lines = append(lines, logLine{stamp: stamp, text: text, offset: offset})
		offset += int64(len(raw))
	}
	return lines, start, offset, size, nil
}

// notes above and below the loaded part of a long file.
const (
	earlierNote = "… earlier output not loaded · g loads the start"
	laterNote   = "… later output not loaded · G loads the end"
)

// targetRow returns the content row of the line jumped to.
func (v logView) targetRow() int {
	if v.target < 0 {
		return 0
	}
	if !v.atStart() {
		return v.target + 1 // below earlierNote
	}
	return v.target
}

// content draws the loaded lines at width.
func (v logView) content(width int) string {
	switch {
	case v.err != nil:
		return ui.Muted.Render("Could not read the log: " + v.err.Error())
	case v.path() == "":
		return ui.Muted.Render("No agent runs logged for this repo yet.")
	}
	var rows []string
	if !v.atStart() {
		rows = append(rows, ui.Muted.Render(earlierNote))
	}
	textWidth := max(width-stampWidth-2, 1)
	raw := ui.Fg(ui.RawText)
	for i, l := range v.lines {
		stamp := fmt.Sprintf("%-*s", stampWidth, l.stamp)
		text := ansi.Truncate(cleanLogText(l.text), textWidth, "")
		if i == v.target {
			text = ansi.Truncate(text+"      ← you jumped here from Activity", textWidth, "")
			rows = append(rows, ui.Bold.Background(ui.SelectBg).Foreground(ui.White).Width(width).
				Render(stamp+"  "+text))
			continue
		}
		rows = append(rows, ui.Muted.Render(stamp+"  ")+raw.Render(text))
	}
	if !v.atEnd() {
		rows = append(rows, ui.Muted.Render(laterNote))
	}
	return strings.Join(rows, "\n")
}

// title is the /log pane's title: "Raw log · Codex · turn 5 (run 12 of 12)".
func (v logView) title() string {
	if v.path() == "" {
		return "Raw log"
	}
	r := v.runs[v.run]
	return fmt.Sprintf("Raw log · %s · turn %d (run %d of %d)", r.agent, r.turn, v.run+1, len(v.runs))
}

// cleanLogText makes a raw output line safe to draw: colour codes and
// control characters are removed (they would mess up the screen), and tabs
// become spaces.
func cleanLogText(s string) string {
	s = strings.ReplaceAll(ansi.Strip(s), "\t", "    ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// runNameRe matches a log file name from runner.LogPath and captures the
// turn number: "20261013-152451-turn1-dev.log".
var runNameRe = regexp.MustCompile(`-turn(\d+)-[a-z]+\.log$`)

// findRuns lists the agent runs logged in logDir for the repo cloned at
// repoDir, oldest first. Every run's log starts with a header line that
// names its folder ("# huddle: codex exec ... in <dir> at <time>"); runs
// of other repos are left out.
func findRuns(logDir, repoDir string) ([]logRun, error) {
	paths, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths) // names start with the date and time
	var runs []logRun
	for _, p := range paths {
		header, err := firstLine(p)
		if err != nil {
			continue // unreadable: not one we can show
		}
		_, header = runner.SplitStamp(header)
		if !strings.HasPrefix(header, "# huddle: ") || !strings.Contains(header, " in "+repoDir+" at ") {
			continue
		}
		run := logRun{path: p, agent: "Codex"}
		if strings.HasPrefix(header, "# huddle: claude ") {
			run.agent = "Claude Code"
		}
		if m := runNameRe.FindStringSubmatch(p); m != nil {
			run.turn, _ = strconv.Atoi(m[1]) // the pattern only matches digits
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// firstLine reads the first line of the file at path.
func firstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, 64*1024))
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSuffix(line, "\n"), nil
}
