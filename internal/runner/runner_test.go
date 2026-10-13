package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeAgent writes an executable shell script and returns its path.
func fakeAgent(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunNormalExit(t *testing.T) {
	dir := t.TempDir()
	agent := fakeAgent(t, `
echo "cwd=$(pwd)"
echo "stdin=$(cat)"
echo "to stderr" >&2
echo '{"result":"OK"}'
`)
	logPath := filepath.Join(t.TempDir(), "logs", "turn1-dev.log")
	var stream bytes.Buffer

	res, err := Run(context.Background(), Spec{
		Name: agent, Dir: dir, Stdin: "the prompt", LogPath: logPath, Stream: &stream,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.ExitCode != 0 || res.TimedOut || res.Canceled {
		t.Errorf("Run() = exit %d, timedOut %t, canceled %t; want 0, false, false", res.ExitCode, res.TimedOut, res.Canceled)
	}
	realDir, _ := filepath.EvalSymlinks(dir)
	for _, want := range []string{"cwd=" + realDir, "stdin=the prompt", `{"result":"OK"}`} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("Stdout missing %q:\n%s", want, res.Stdout)
		}
	}
	if strings.Contains(res.Stdout, "to stderr") {
		t.Errorf("Stdout contains stderr output:\n%s", res.Stdout)
	}
	for name, got := range map[string]string{"stream": stream.String(), "tail": res.Tail} {
		if !strings.Contains(got, "to stderr") || !strings.Contains(got, "stdin=the prompt") {
			t.Errorf("%s missing stdout or stderr:\n%s", name, got)
		}
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# huddle: ", "(prompt, 10 bytes)", "to stderr", `{"result":"OK"}`, "exit=0 timed_out=false"} {
		if !strings.Contains(string(log), want) {
			t.Errorf("log missing %q:\n%s", want, log)
		}
	}
}

func TestRunNonZeroExit(t *testing.T) {
	agent := fakeAgent(t, "echo failing; exit 3\n")
	res, err := Run(context.Background(), Spec{Name: agent, Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (non-zero exit is a result)", err)
	}
	if res.ExitCode != 3 || res.TimedOut || res.Canceled {
		t.Errorf("Run() = exit %d, timedOut %t, canceled %t; want 3, false, false", res.ExitCode, res.TimedOut, res.Canceled)
	}
	if res.Tail != "failing" {
		t.Errorf("Tail = %q, want %q", res.Tail, "failing")
	}
}

func TestRunMissingBinary(t *testing.T) {
	_, err := Run(context.Background(), Spec{Name: "huddle-no-such-agent", Dir: t.TempDir()})
	if err == nil {
		t.Fatal("Run() error = nil, want an error for a missing binary")
	}
}

// groupAgent starts a grandchild that would outlive a plain kill of the
// leader, records both pids, then hangs.
func groupAgent(t *testing.T) (agent, pidFile string) {
	pidFile = filepath.Join(t.TempDir(), "pids")
	agent = fakeAgent(t, `
sleep 300 &
echo "$$ $!" > "`+pidFile+`"
echo started
wait
`)
	return agent, pidFile
}

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	agent, pidFile := groupAgent(t)
	start := time.Now()
	res, err := Run(context.Background(), Spec{Name: agent, Dir: t.TempDir(), Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !res.TimedOut || res.Canceled {
		t.Errorf("TimedOut = %t, Canceled = %t; want true, false", res.TimedOut, res.Canceled)
	}
	if res.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 (killed)", res.ExitCode)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("Run() took %s, want it to return soon after the timeout", d)
	}
	if res.Tail != "started" {
		t.Errorf("Tail = %q, want %q", res.Tail, "started")
	}
	assertAllDead(t, pidFile)
}

func TestRunNoTimeoutByDefault(t *testing.T) {
	agent := fakeAgent(t, "sleep 2; echo finished\n")
	res, err := Run(context.Background(), Spec{Name: agent, Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.ExitCode != 0 || res.TimedOut || res.Canceled {
		t.Errorf("Run() = exit %d, timedOut %t, canceled %t; want 0, false, false", res.ExitCode, res.TimedOut, res.Canceled)
	}
	if res.Tail != "finished" {
		t.Errorf("Tail = %q, want %q", res.Tail, "finished")
	}
}

func TestRunCancelKillsProcessGroup(t *testing.T) {
	agent, pidFile := groupAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	res, err := Run(ctx, Spec{Name: agent, Dir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !res.Canceled || res.TimedOut {
		t.Errorf("Canceled = %t, TimedOut = %t; want true, false", res.Canceled, res.TimedOut)
	}
	assertAllDead(t, pidFile)
}

func TestRunNormalExitKillsProcessGroup(t *testing.T) {
	old := waitDelay
	waitDelay = 500 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })

	for _, tt := range []struct {
		name     string
		redirect string
	}{
		{"leftover detached from output", ">/dev/null 2>&1"},
		{"leftover holding output pipes", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pidFile := filepath.Join(t.TempDir(), "pids")
			agent := fakeAgent(t, `
sleep 300 `+tt.redirect+` &
echo "$$ $!" > "`+pidFile+`"
echo done
exit 0
`)
			start := time.Now()
			res, err := Run(context.Background(), Spec{Name: agent, Dir: t.TempDir()})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if res.ExitCode != 0 || res.TimedOut || res.Canceled {
				t.Errorf("Run() = exit %d, timedOut %t, canceled %t; want 0, false, false", res.ExitCode, res.TimedOut, res.Canceled)
			}
			if d := time.Since(start); d > 4*time.Second {
				t.Errorf("Run() took %s, want it to return soon after the agent exits", d)
			}
			if res.Tail != "done" {
				t.Errorf("Tail = %q, want %q", res.Tail, "done")
			}
			assertAllDead(t, pidFile)
		})
	}
}

// assertAllDead checks that every pid in pidFile is gone (or a zombie
// awaiting its reaper) within a couple of seconds.
func assertAllDead(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		t.Fatalf("pid file = %q, want two pids", data)
	}
	for _, f := range fields {
		pid, err := strconv.Atoi(f)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for alive(pid) {
			if time.Now().After(deadline) {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Errorf("process %d still running after the run was killed", pid)
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// Field 3, after the parenthesised command name, is the state.
	if i := bytes.LastIndexByte(stat, ')'); i >= 0 && i+2 < len(stat) {
		return stat[i+2] != 'Z'
	}
	return true
}

func TestRunStripsAPIKeys(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("HUDDLE_TEST_KEEP", "kept")
	agent := fakeAgent(t, `
for v in OPENAI_API_KEY ANTHROPIC_API_KEY HUDDLE_TEST_KEEP; do
  if env | grep -q "^$v="; then echo "$v=visible"; else echo "$v=absent"; fi
done
`)
	for _, tt := range []struct {
		name string
		env  []string
	}{
		{"inherited environment", nil},
		{"explicit environment", []string{"OPENAI_API_KEY=a", "ANTHROPIC_API_KEY=b", "HUDDLE_TEST_KEEP=kept", "PATH=" + os.Getenv("PATH")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Run(context.Background(), Spec{Name: agent, Dir: t.TempDir(), Env: tt.env})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			want := "OPENAI_API_KEY=absent\nANTHROPIC_API_KEY=absent\nHUDDLE_TEST_KEEP=visible\n"
			if res.Stdout != want {
				t.Errorf("child saw:\n%s\nwant:\n%s", res.Stdout, want)
			}
		})
	}
}

func TestStripEnv(t *testing.T) {
	got := StripEnv([]string{"A=1", "OPENAI_API_KEY=x", "OPENAI_API_KEY_OLD=y", "ANTHROPIC_API_KEY=", "B=2"})
	want := []string{"A=1", "OPENAI_API_KEY_OLD=y", "B=2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("StripEnv() = %v, want %v", got, want)
	}
}

func TestTailWriter(t *testing.T) {
	tests := []struct {
		name   string
		writes []string
		max    int
		want   string
	}{
		{"empty", nil, 3, ""},
		{"fewer than max", []string{"a\nb\n"}, 3, "a\nb"},
		{"more than max", []string{"1\n2\n3\n4\n5\n"}, 3, "3\n4\n5"},
		{"split across writes", []string{"hel", "lo\nwor", "ld\n"}, 3, "hello\nworld"},
		{"trailing partial line", []string{"1\n2\n3\npartial"}, 3, "2\n3\npartial"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tw := &tailWriter{max: tt.max}
			for _, w := range tt.writes {
				tw.Write([]byte(w))
			}
			if got := tw.String(); got != tt.want {
				t.Errorf("tail = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHeadWriter(t *testing.T) {
	tests := []struct {
		name   string
		writes []string
		max    int
		want   string
	}{
		{"empty", nil, 3, ""},
		{"fewer than max", []string{"a\nb\n"}, 3, "a\nb"},
		{"more than max", []string{"1\n2\n3\n4\n5\n"}, 3, "1\n2\n3"},
		{"split across writes", []string{"hel", "lo\nwor", "ld\nmore\nlines\n"}, 2, "hello\nworld"},
		{"trailing partial line", []string{"1\npart"}, 3, "1\npart"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &headWriter{max: tt.max}
			for _, w := range tt.writes {
				hw.Write([]byte(w))
			}
			if got := hw.String(); got != tt.want {
				t.Errorf("head = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLogPath(t *testing.T) {
	ts := time.Date(2026, 10, 12, 17, 5, 9, 0, time.UTC)
	got := LogPath("/home/u", ts, 3, "pm")
	if want := "/home/u/.huddle/logs/20261012-170509-turn3-pm.log"; got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
}
