// Package runner runs an agent CLI with a timeout, process-group kill, env
// stripping and logging.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
)

// DefaultTimeout is the limit for one agent run (section 8 of the plan).
const DefaultTimeout = 30 * time.Minute

// DefaultTailLines is how many lines of output Result.Tail keeps.
const DefaultTailLines = 50

// waitDelay bounds how long Run waits for output pipes to close after the
// agent exits or is killed, in case a descendant still holds them. It is a
// variable so tests can shorten it.
var waitDelay = 5 * time.Second

// Spec describes one agent run.
type Spec struct {
	Name    string   // binary, looked up on PATH
	Args    []string // arguments
	Stdin   string   // passed on stdin; empty means no stdin
	Dir     string   // working directory
	Timeout time.Duration
	LogPath string    // full output is written here; empty means no log file
	Stream  io.Writer // live output, normally os.Stdout; nil means none
	Tail    int       // lines kept in Result.Tail; 0 means DefaultTailLines
	Env     []string  // base environment; nil means os.Environ()
}

// Result is the outcome of a run that started.
type Result struct {
	ExitCode int    // -1 if the process was killed by a signal
	TimedOut bool   // killed because Spec.Timeout passed
	Canceled bool   // killed because the parent context was canceled
	Stdout   string // stdout only, in full (the PM's JSON envelope)
	Tail     string // last Spec.Tail lines of combined stdout and stderr
	Duration time.Duration
}

// Run runs spec. A non-zero exit, timeout or cancel is reported in Result,
// not as an error; the error is only for runs that could not start.
// The whole process group is killed when the run ends, however it ends.
func Run(ctx context.Context, spec Spec) (Result, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	tailN := spec.Tail
	if tailN <= 0 {
		tailN = DefaultTailLines
	}
	env := spec.Env
	if env == nil {
		env = os.Environ()
	}

	var logFile *os.File
	if spec.LogPath != "" {
		if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o755); err != nil {
			return Result{}, fmt.Errorf("create log dir: %w", err)
		}
		f, err := os.Create(spec.LogPath)
		if err != nil {
			return Result{}, fmt.Errorf("create log: %w", err)
		}
		defer f.Close()
		logFile = f
		fmt.Fprintf(f, "# huddle: %s in %s at %s\n", commandLine(spec), spec.Dir, time.Now().Format(time.RFC3339))
	}

	tail := &tailWriter{max: tailN}
	writers := []io.Writer{tail}
	if spec.Stream != nil {
		writers = append(writers, spec.Stream)
	}
	if logFile != nil {
		writers = append(writers, logFile)
	}
	combined := &lockedWriter{w: io.MultiWriter(writers...)}
	var stdout bytes.Buffer

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, spec.Name, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = StripEnv(env)
	if spec.Stdin != "" {
		cmd.Stdin = strings.NewReader(spec.Stdin)
	}
	cmd.Stdout = io.MultiWriter(&stdout, combined)
	cmd.Stderr = combined
	// Own process group, so the terminal's Ctrl+C reaches only Huddle and
	// Huddle can kill the agent together with everything it spawned.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return killGroup(cmd.Process.Pid) }
	cmd.WaitDelay = waitDelay

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("start %s: %w", spec.Name, err)
	}
	waitErr := cmd.Wait()
	// Always kill the group, including after a normal exit: anything the
	// agent left running in the background must not outlive the turn. A
	// leftover that holds the output pipes delays Wait by up to waitDelay.
	// The pid cannot be reused while the group still has members.
	_ = killGroup(cmd.Process.Pid)

	res := Result{
		ExitCode: cmd.ProcessState.ExitCode(),
		Stdout:   stdout.String(),
		Tail:     tail.String(),
		Duration: time.Since(start),
	}
	if runCtx.Err() != nil {
		if ctx.Err() != nil {
			res.Canceled = true
		} else {
			res.TimedOut = true
		}
	}
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) && !errors.Is(waitErr, exec.ErrWaitDelay) && runCtx.Err() == nil {
		return res, fmt.Errorf("wait for %s: %w", spec.Name, waitErr)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "\n# huddle: exit=%d timed_out=%t canceled=%t duration=%s\n",
			res.ExitCode, res.TimedOut, res.Canceled, res.Duration.Round(time.Second))
	}
	return res, nil
}

// LogPath returns the log file path for a turn:
// <home>/.huddle/logs/<timestamp>-turn<N>-<role>.log.
func LogPath(home string, t time.Time, turn int, role string) string {
	name := fmt.Sprintf("%s-turn%d-%s.log", t.Format("20060102-150405"), turn, role)
	return filepath.Join(home, ".huddle", "logs", name)
}

// StripEnv returns env without the API key variables, so an agent can never
// fall back to API billing.
func StripEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !isAPIKeyVar(name) {
			out = append(out, kv)
		}
	}
	return out
}

func isAPIKeyVar(name string) bool {
	for _, v := range checks.APIKeyVars {
		if name == v {
			return true
		}
	}
	return false
}

// killGroup sends SIGKILL to the process group led by pid.
func killGroup(pid int) error {
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}

// commandLine renders the command for the log header. The prompt goes on
// stdin, so it is not repeated here.
func commandLine(spec Spec) string {
	parts := append([]string{spec.Name}, spec.Args...)
	for i, p := range parts {
		if p == "" || strings.ContainsAny(p, " \t\n\"'*()") {
			parts[i] = fmt.Sprintf("%q", p)
		}
	}
	s := strings.Join(parts, " ")
	if spec.Stdin != "" {
		s += fmt.Sprintf(" < (prompt, %d bytes)", len(spec.Stdin))
	}
	return s
}

// lockedWriter serialises writes from the stdout and stderr copiers.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// tailWriter keeps the last max lines written to it.
type tailWriter struct {
	max     int
	lines   []string
	partial []byte
}

func (t *tailWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			t.partial = append(t.partial, p...)
			break
		}
		t.partial = append(t.partial, p[:i]...)
		t.lines = append(t.lines, string(t.partial))
		t.partial = t.partial[:0]
		if len(t.lines) > t.max {
			t.lines = t.lines[len(t.lines)-t.max:]
		}
		p = p[i+1:]
	}
	return n, nil
}

func (t *tailWriter) String() string {
	lines := t.lines
	if len(t.partial) > 0 {
		lines = append(append([]string(nil), lines...), string(t.partial))
		if len(lines) > t.max {
			lines = lines[len(lines)-t.max:]
		}
	}
	return strings.Join(lines, "\n")
}
