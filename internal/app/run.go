package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
)

// EventMsg carries an Activity event. The reporter sends it, from the
// loop's goroutine, for every event the loop or an agent reports.
type EventMsg activity.Event

// StatusMsg carries a status change. The reporter sends it, from the
// loop's goroutine, whenever the loop's phase changes.
type StatusMsg loop.Status

// LoopDoneMsg is sent by Run when the loop has ended, with its exit code.
type LoopDoneMsg struct{ Code int }

// reporter passes the loop's reports to the Bubble Tea program as
// messages. The loop calls it from its own goroutine; Program.Send is safe
// to call from any goroutine and hands the message to Update, the only
// place that may change the screen's state.
type reporter struct {
	program *tea.Program
}

func (r reporter) Event(e activity.Event) { r.program.Send(EventMsg(e)) }
func (r reporter) Status(s loop.Status)   { r.program.Send(StatusMsg(s)) }

// Run shows the run screen while l runs, and returns l's exit code. The
// terminal is taken over until the human quits (/quit or Ctrl+C) and given
// back afterwards. l.Report and l.Controls are set here; cfg.Controls and
// cfg.Stop must not be set by the caller.
//
// stderr receives any error the screen itself hits.
func Run(ctx context.Context, cfg Config, l *loop.Loop, stderr io.Writer) int {
	// The loop gets its own context, so /quit can stop it while the
	// program keeps running until the loop has put its label back.
	loopCtx, stop := context.WithCancel(ctx)
	defer stop()
	cfg.Stop = stop
	cfg.Controls = &loop.Controls{}
	l.Controls = cfg.Controls

	// WithAltScreen draws on the terminal's alternate screen, like vim or
	// less, so the shell's scrollback is back as it was afterwards.
	// WithMouseCellMotion turns on mouse events (wheel and clicks).
	p := tea.NewProgram(New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	l.Report = reporter{program: p}

	// The loop runs on its own goroutine: Bubble Tea's p.Run below blocks
	// this one until the screen closes. done is closed when the loop ends.
	done := make(chan struct{})
	code := loop.ExitInterrupted
	go func() {
		defer close(done)
		code = l.Run(loopCtx)
		p.Send(LoopDoneMsg{Code: code})
	}()

	// A program stopped by a signal (Ctrl+C from outside, or SIGTERM) is
	// not an error worth printing: the loop is stopped below either way.
	_, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
		fmt.Fprintf(stderr, "huddle: %v\n", err)
	}
	// Normally the loop has already ended (see /quit in commands.go). If
	// the screen closed another way, stop the loop now and wait for it to
	// put back its label. Reading code after <-done is safe: closing done
	// happens after the goroutine's last write to code.
	stop()
	<-done
	return code
}
