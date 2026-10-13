package runner

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"time"
)

// Every line in a log file starts with the time it arrived, in this layout,
// followed by two spaces. The time lets the /log view show when each line
// was written; the Activity pane needs it too.
const (
	StampLayout = "15:04:05"
	stampSep    = "  "
)

// Line is one finished line of an agent's output, as written to its log file.
type Line struct {
	Text   string    // the line without its time stamp and newline
	Time   time.Time // when the line arrived
	Offset int64     // byte offset in the log file where the line starts
}

// logWriter writes agent output to a log file, one whole line at a time,
// with each line prefixed by the time it arrived. After writing a line it
// hands the line to onLine (if set), which is how the Activity pane learns
// about the agent's progress while it runs.
//
// It is called from the runner's stdout and stderr copiers, but always
// under lockedWriter's lock, so it needs no lock of its own.
type logWriter struct {
	file    io.Writer
	now     func() time.Time
	onLine  func(Line)
	offset  int64  // bytes written to file so far
	partial []byte // the start of a line whose newline has not arrived yet
}

func (w *logWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			w.partial = append(w.partial, p...)
			break
		}
		w.partial = append(w.partial, p[:i]...)
		if err := w.writeLine(string(w.partial)); err != nil {
			return 0, err
		}
		w.partial = w.partial[:0]
		p = p[i+1:]
	}
	return n, nil
}

// Flush writes out a last line that never got its newline.
func (w *logWriter) Flush() error {
	if len(w.partial) == 0 {
		return nil
	}
	err := w.writeLine(string(w.partial))
	w.partial = w.partial[:0]
	return err
}

func (w *logWriter) writeLine(text string) error {
	// A carriage return would move the cursor back over the time stamp when
	// the file is printed, so it is dropped.
	text = strings.TrimSuffix(text, "\r")
	t := w.now()
	line := Line{Text: text, Time: t, Offset: w.offset}
	n, err := io.WriteString(w.file, t.Format(StampLayout)+stampSep+text+"\n")
	w.offset += int64(n)
	if err != nil {
		return err
	}
	if w.onLine != nil {
		w.onLine(line)
	}
	return nil
}

// stampRe matches the time stamp at the start of a log line. An empty
// line is a stamp alone, and editors often trim its two trailing spaces.
var stampRe = regexp.MustCompile(`^\d\d:\d\d:\d\d(?:  |$)`)

// SplitStamp splits a log file line into its time stamp ("15:04:05") and
// text. Logs written before Huddle v3 have no stamps; their lines come back
// with an empty stamp.
func SplitStamp(line string) (stamp, text string) {
	if !stampRe.MatchString(line) {
		return "", line
	}
	stamp = line[:len(StampLayout)]
	text = strings.TrimPrefix(line[len(StampLayout):], stampSep)
	return stamp, text
}

// StripStamps removes the time stamp from every line of a log file's
// content, giving back the agent's raw output. The usage-limit checks need
// that: they look for messages at the start of a line.
func StripStamps(log string) string {
	lines := strings.Split(log, "\n")
	for i, l := range lines {
		_, lines[i] = SplitStamp(l)
	}
	return strings.Join(lines, "\n")
}
