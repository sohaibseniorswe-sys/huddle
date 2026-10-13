package runner

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogWriterStampsLinesAndReportsOffsets(t *testing.T) {
	var file bytes.Buffer
	var got []Line
	at := time.Date(2026, 10, 13, 22, 52, 7, 0, time.UTC)
	w := &logWriter{file: &file, now: func() time.Time { return at }, onLine: func(l Line) { got = append(got, l) }}

	// Lines arrive in pieces, as they do from a pipe.
	for _, piece := range []string{"exec\n/bin/ba", "sh -lc 'go test'\r\n", "no newline"} {
		w.Write([]byte(piece))
	}
	if len(got) != 2 {
		t.Fatalf("lines before Flush = %d, want 2 (the last has no newline yet)", len(got))
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}

	want := "22:52:07  exec\n22:52:07  /bin/bash -lc 'go test'\n22:52:07  no newline\n"
	if file.String() != want {
		t.Errorf("file = %q, want %q", file.String(), want)
	}
	wantTexts := []string{"exec", "/bin/bash -lc 'go test'", "no newline"}
	for i, l := range got {
		if l.Text != wantTexts[i] {
			t.Errorf("line %d text = %q, want %q", i, l.Text, wantTexts[i])
		}
		// The offset must point at the start of that line in the file.
		if rest := file.String()[l.Offset:]; !strings.HasPrefix(rest, "22:52:07  "+wantTexts[i]+"\n") {
			t.Errorf("line %d offset %d points at %q", i, l.Offset, rest)
		}
	}
}

func TestSplitAndStripStamps(t *testing.T) {
	for _, tt := range []struct{ line, stamp, text string }{
		{"22:52:07  exec", "22:52:07", "exec"},
		{"22:52:07  ", "22:52:07", ""},
		{"22:52:07", "22:52:07", ""},                     // an empty line whose spaces were trimmed
		{"exec", "", "exec"},                             // a log from before v3
		{"22:52:07 one space", "", "22:52:07 one space"}, // not a stamp
	} {
		stamp, text := SplitStamp(tt.line)
		if stamp != tt.stamp || text != tt.text {
			t.Errorf("SplitStamp(%q) = %q, %q; want %q, %q", tt.line, stamp, text, tt.stamp, tt.text)
		}
	}
	if got := StripStamps("10:00:00  a\n10:00:01  ERROR: b\nc"); got != "a\nERROR: b\nc" {
		t.Errorf("StripStamps() = %q", got)
	}
}

func TestRunReportsLogLines(t *testing.T) {
	agent := fakeAgent(t, "echo one; echo two >&2\n")
	logPath := filepath.Join(t.TempDir(), "run.log")
	var texts []string
	_, err := Run(context.Background(), Spec{
		Name: agent, Dir: t.TempDir(), LogPath: logPath,
		OnLine: func(l Line) { texts = append(texts, l.Text) },
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"# huddle: ", "one", "two", "# huddle: exit=0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("OnLine lines miss %q:\n%s", want, joined)
		}
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if stamp, _ := SplitStamp(line); stamp == "" {
			t.Errorf("log line without a time stamp: %q", line)
		}
	}
}
