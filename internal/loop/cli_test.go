package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

// fakeBin writes an executable script named name into a fresh PATH dir.
func fakeBin(t *testing.T, scripts map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCLIAgentsDev(t *testing.T) {
	// Like codex exec: header on stderr, final message to --output-last-message.
	fakeBin(t, map[string]string{"codex": `
while [ $# -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then last=$2; fi
  shift
done
cat > /dev/null
printf 'OpenAI Codex v0.160.0\n--------\nsession id: 01a1-dev\n--------\n' >&2
echo "ERROR: You’ve hit your usage limit. Try again later." >&2
printf 'codex\nClosed #1. All tests pass.\n\ntokens used\n100\n' >&2
printf 'Done: closed #1' > "$last"
echo 'Done: closed #1'
exit 1
`})
	home := t.TempDir()
	var events []activity.Event
	a := CLIAgents{Dir: t.TempDir(), Home: home, Team: defaultTeam()}
	res, err := a.Run(context.Background(), session.Dev, "", "prompt", 7, func(e activity.Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 1 || res.SessionID != "01a1-dev" || res.FinalMessage != "Done: closed #1" {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(res.Output, "You’ve hit your usage limit") {
		t.Errorf("Output misses the limit message:\n%s", res.Output)
	}
	l := &Loop{Clock: RealClock{}, Team: defaultTeam()}
	if u := l.usageLimit(session.Dev, res.Output); !u.Hit {
		t.Errorf("usage limit not found in the time-stamped log: %+v", u)
	}
	logs, _ := filepath.Glob(filepath.Join(home, ".huddle", "logs", "*-turn7-dev.log"))
	if len(logs) != 1 {
		t.Fatalf("logs = %v, want one turn7-dev log", logs)
	}
	if len(events) != 1 || events[0].Text != "Closed #1" || events[0].Actor != activity.ActorCodex || events[0].LogRef.Path != logs[0] {
		t.Errorf("events = %+v, want one codex event pointing into %s", events, logs[0])
	}
}

func TestCLIAgentsPM(t *testing.T) {
	fakeBin(t, map[string]string{"claude": `
cat > /dev/null
echo "$@" >&2
echo '{"type":"result","subtype":"success","is_error":true,"session_id":"pm-42","result":"You'"'"'ve hit your session limit · resets 3pm (Europe/London)"}'
`})
	a := CLIAgents{Dir: t.TempDir(), Home: t.TempDir(), Team: defaultTeam()}
	res, err := a.Run(context.Background(), session.PM, "pm-41", "prompt", 1, func(activity.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID != "pm-42" || !strings.HasPrefix(res.FinalMessage, "You've hit your session limit") {
		t.Errorf("result = %+v", res)
	}
	if !strings.Contains(res.Output, "--resume pm-41") {
		t.Errorf("not resumed:\n%s", res.Output)
	}
	l := &Loop{Clock: RealClock{}, Team: defaultTeam()}
	if u := l.usageLimit(session.PM, res.Output); !u.Hit || u.ResetAt.IsZero() {
		t.Errorf("usage limit in the envelope not found: %+v", u)
	}
}

// TestCLIAgentsSwappedRoles runs Claude Code as the Dev and Codex as the
// PM: each role gets its CLI's command with the model and effort, and the
// output is read the way that CLI writes it.
func TestCLIAgentsSwappedRoles(t *testing.T) {
	fakeBin(t, map[string]string{
		"claude": `
cat > /dev/null
echo "$@" >&2
echo '{"type":"result","subtype":"success","is_error":false,"session_id":"dev-claude-1","result":"Merged PR #3, closed #2."}'
`,
		"codex": `
echo "$@" >&2
while [ $# -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then last=$2; fi
  shift
done
cat > /dev/null
printf 'OpenAI Codex v0.160.0\n--------\nmodel: gpt-6.1-sol\nsession id: pm-codex-1\n--------\n' >&2
printf 'codex\nPosted the next prompt.\n' >&2
printf 'Set state:dev-ready' > "$last"
`,
	})
	home := t.TempDir()
	a := CLIAgents{Dir: t.TempDir(), Home: home, Team: swappedTeam()}

	var events []activity.Event
	dev, err := a.Run(context.Background(), session.Dev, "", "prompt", 1, func(e activity.Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if dev.SessionID != "dev-claude-1" || dev.FinalMessage != "Merged PR #3, closed #2." {
		t.Errorf("Dev (Claude Code) result = %+v", dev)
	}
	if !strings.Contains(dev.Output, "-p --output-format json --dangerously-skip-permissions --model sonnet --effort high") {
		t.Errorf("Dev command lacks Claude Code's flags:\n%s", dev.Output)
	}
	if len(events) != 1 || events[0].Actor != activity.ActorClaude {
		t.Errorf("Dev events = %+v, want one claude event", events)
	}

	events = nil
	pm, err := a.Run(context.Background(), session.PM, "pm-codex-0", "prompt", 2, func(e activity.Event) { events = append(events, e) })
	if err != nil {
		t.Fatal(err)
	}
	if pm.SessionID != "pm-codex-1" || pm.FinalMessage != "Set state:dev-ready" {
		t.Errorf("PM (Codex) result = %+v", pm)
	}
	if !strings.Contains(pm.Output, `exec resume --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" -m gpt-6.1-sol -c model_reasoning_effort="xhigh"`) ||
		!strings.Contains(pm.Output, "pm-codex-0 -") {
		t.Errorf("PM command lacks Codex's resume flags:\n%s", pm.Output)
	}
	if len(events) != 1 || events[0].Actor != activity.ActorCodex {
		t.Errorf("PM events = %+v, want one codex event", events)
	}
	logs, _ := filepath.Glob(filepath.Join(home, ".huddle", "logs", "*-turn2-pm.log"))
	if len(logs) != 1 {
		t.Errorf("logs = %v, want one turn2-pm log", logs)
	}

	// Usage limits are read with the CLI that plays the role: a Codex
	// message counts for the PM now, a Claude Code one for the Dev.
	l := &Loop{Clock: RealClock{}, Team: swappedTeam()}
	if u := l.usageLimit(session.PM, "ERROR: You’ve hit your usage limit. Try again later."); !u.Hit {
		t.Errorf("Codex limit for the PM not found: %+v", u)
	}
	if u := l.usageLimit(session.Dev, "You've hit your session limit · resets 3pm (Europe/London)"); !u.Hit {
		t.Errorf("Claude Code limit for the Dev not found: %+v", u)
	}
}
