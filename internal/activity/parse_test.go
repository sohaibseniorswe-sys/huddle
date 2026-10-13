package activity

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
)

// feed sends output to a parser line by line, as runner.Run does. Offsets
// count the bytes of the lines as given (the test files have no stamps).
func feed(output string, line func(runner.Line)) {
	start := time.Date(2026, 10, 13, 22, 0, 0, 0, time.UTC)
	var offset int64
	for i, text := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		line(runner.Line{Text: text, Time: start.Add(time.Duration(i) * time.Second), Offset: offset})
		offset += int64(len(text)) + 1
	}
}

func TestCodex(t *testing.T) {
	data, err := os.ReadFile("testdata/codex.out")
	if err != nil {
		t.Fatal(err)
	}
	var got []Event
	p := NewCodex("turn1-dev.log", func(e Event) { got = append(got, e) })
	feed(string(data), p.Line)
	p.Flush()

	want := []struct {
		text     string
		lineText string // the log line the event's LogRef points at
	}{
		{"I’ll read the control issue history, milestone issues, and repository instructions", "I’ll read the control"},
		{"$ git status --short   ok", "/bin/bash -lc 'git status --short'"},
		{"$ gh issue view 12 --comments   exit 1", `/bin/bash -lc "gh issue view 12`},
		{"The default `gh issue view` command failed on a deprecated GitHub field; I’ll use explicit JSON fields", "The default"},
		{"PR #13 is awaiting CI", "PR #13 is awaiting CI."},
		{"$ gh pr checks 13 --watch --interval 5   ok", "/bin/bash -lc 'gh pr checks 13"},
		{"Closed #7–#10 through merged PRs #13–#16", "Closed #7–#10"},
		{"#11 still requires personal maintainer approval", "#11 still requires"},
		{"$ sleep 100   (no result)", "/bin/bash -lc 'sleep 100'"},
	}
	if len(got) != len(want) {
		for _, e := range got {
			t.Logf("got %q", e.Text)
		}
		t.Fatalf("got %d events, want %d", len(got), len(want))
	}
	for i, w := range want {
		e := got[i]
		if e.Text != w.text || e.Actor != ActorCodex {
			t.Errorf("event %d = %s %q, want codex %q", i, e.Actor, e.Text, w.text)
		}
		if e.LogRef.Path != "turn1-dev.log" || !strings.HasPrefix(string(data[e.LogRef.Offset:]), w.lineText) {
			t.Errorf("event %d LogRef %+v points at %.40q, want %q", i, e.LogRef, data[e.LogRef.Offset:], w.lineText)
		}
	}
}

func TestClaude(t *testing.T) {
	output := `# huddle: claude -p --output-format json in /tmp at 2026-10-13T15:30:50-04:00
some stray stderr line
{"type":"result","is_error":false,"session_id":"717c","result":"Merged PR #71 and closed #61. Next is #62."}
{"type":"result","is_error":true,"result":"You've hit your session limit · resets 3pm"}
{not json`
	var got []Event
	p := NewClaude("turn2-pm.log", func(e Event) { got = append(got, e) })
	feed(output, p.Line)
	p.Flush()
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(got), got)
	}
	if got[0].Text != "Merged PR #71 and closed #61" || got[0].Actor != ActorClaude {
		t.Errorf("event 0 = %+v", got[0])
	}
	if !strings.HasPrefix(output[got[0].LogRef.Offset:], `{"type":"result","is_error":false`) {
		t.Errorf("event 0 LogRef points at %.30q", output[got[0].LogRef.Offset:])
	}
	if got[1].Text != "Error: You've hit your session limit · resets 3pm" {
		t.Errorf("event 1 = %q", got[1].Text)
	}
}

func TestFirstSentence(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"Merged PR #71, closed #61.", "Merged PR #71, closed #61"},
		{"Read AGENTS.md and #58.  Then start.", "Read AGENTS.md and #58"},
		{"Done!\nNext?", "Done!"},
		{"  spread\n over\tlines  ", "spread over lines"},
		{strings.Repeat("a", 300), strings.Repeat("a", maxText-1) + "…"},
	} {
		if got := FirstSentence(tt.in); got != tt.want {
			t.Errorf("FirstSentence(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCommandText(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{`/bin/bash -lc 'go test ./...' in /home/u/p`, "go test ./..."},
		{`/bin/bash -lc "git push && gh pr create --title 'x'" in /home/u/p`, "git push && gh pr create --title 'x'"},
		{`/bin/bash -lc "cat <<'EOF' > f`, `"cat <<'EOF' > f`}, // a heredoc continues on the next lines
		{"/bin/bash -lc '" + strings.Repeat("x", 100) + "' in /p", strings.Repeat("x", maxCommand-1) + "…"},
	} {
		if got := commandText(tt.in); got != tt.want {
			t.Errorf("commandText(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
