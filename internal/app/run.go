package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/activity"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
)

// EventMsg carries an Activity event. The reporter sends it, from the
// loop's goroutine, for every event the loop or an agent reports.
type EventMsg activity.Event

// StatusMsg carries a status change. The reporter sends it, from the
// loop's goroutine, whenever the loop's phase changes.
type StatusMsg loop.Status

// LoopDoneMsg is sent by Run when the loop goroutine has ended, with the
// last loop's exit code. Finished is true when the last milestone is done
// and nothing is queued: the badge says Done, and the next key press
// returns to the start menu.
type LoopDoneMsg struct {
	Code     int
	Finished bool
}

// MilestoneMsg is sent by Run, from the loop goroutine, whenever a
// milestone starts: the first one, and each queued one after it.
type MilestoneMsg struct {
	Milestone string
	Control   gh.Control
}

// reporter passes the loop's reports to the Bubble Tea program as
// messages. The loop calls it from its own goroutine; Program.Send is safe
// to call from any goroutine and hands the message to Update, the only
// place that may change the screen's state.
type reporter struct {
	program *tea.Program
}

func (r reporter) Event(e activity.Event) { r.program.Send(EventMsg(e)) }
func (r reporter) Status(s loop.Status)   { r.program.Send(StatusMsg(s)) }

// Run shows the run screen and runs milestones until the human quits, and
// returns the last loop's exit code. startMenu is true when the last
// milestone finished, nothing was queued, and the human pressed a key to
// go back to the start menu.
//
// Outside plan mode it runs cfg.First first; in plan mode it waits until a
// plan is agreed. After each milestone that reaches state:done it starts
// the next one in cfg.Queue, without asking. The terminal is taken over
// until the human quits (/quit or Ctrl+C) and given back afterwards.
// cfg.Controls, cfg.Stop, cfg.CompanionCtx, cfg.StopCompanion, cfg.Send
// and cfg.Begin are set here, not by the caller.
//
// stderr receives any error the screen itself hits.
func Run(ctx context.Context, cfg Config, stderr io.Writer) (code int, startMenu bool) {
	// The loop gets its own context, so /quit can stop it while the
	// program keeps running until the loop has put its label back.
	loopCtx, stop := context.WithCancel(ctx)
	defer stop()
	cfg.Stop = stop
	cfg.Controls = &loop.Controls{}
	// Companion runs get a context of their own too: /quit cancels it, and
	// so does closing the screen, which kills a companion still thinking.
	companionCtx, stopCompanion := context.WithCancel(ctx)
	defer stopCompanion()
	cfg.CompanionCtx, cfg.StopCompanion = companionCtx, stopCompanion
	if !cfg.Planning {
		cfg.Milestone = cfg.First.Milestone
		cfg.Control.Number = cfg.First.Number
	}

	// begin wakes the loop goroutine in plan mode. It holds at most one
	// signal, so Begin never blocks Update.
	begin := make(chan struct{}, 1)
	cfg.Begin = func() {
		select {
		case begin <- struct{}{}:
		default: // already signalled
		}
	}
	// The program does not exist yet when cfg is handed to New, so Send
	// reaches it through this variable, set just below.
	var p *tea.Program
	cfg.Send = func(msg tea.Msg) { p.Send(msg) }

	// WithAltScreen draws on the terminal's alternate screen, like vim or
	// less, so the shell's scrollback is back as it was afterwards.
	// WithMouseCellMotion turns on mouse events (wheel and clicks).
	p = tea.NewProgram(New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))

	// The loops run on their own goroutine: Bubble Tea's p.Run below
	// blocks this one until the screen closes. done is closed when the
	// last loop ends.
	done := make(chan struct{})
	code = loop.ExitInterrupted
	go func() {
		defer close(done)
		var finished bool
		code, finished = drive(loopCtx, cfg, reporter{program: p}, begin)
		p.Send(LoopDoneMsg{Code: code, Finished: finished})
	}()

	// A program stopped by a signal (Ctrl+C from outside, or SIGTERM) is
	// not an error worth printing: the loop is stopped below either way.
	final, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
		fmt.Fprintf(stderr, "huddle: %v\n", err)
	}
	// Normally the loop has already ended (see /quit in commands.go). If
	// the screen closed another way, stop the loop now and wait for it to
	// put back its label. Reading code after <-done is safe: closing done
	// happens after the goroutine's last write to code.
	stop()
	stopCompanion()
	<-done
	if m, ok := final.(Model); ok {
		startMenu = m.toStart
	}
	return code, startMenu
}

// drive runs on the loop goroutine. It runs one milestone after another:
// cfg.First (or, in plan mode, the first queued one once begin says a plan
// was agreed), then each queued milestone after one reaches state:done.
// It returns the last loop's exit code, and finished = true when the last
// milestone is done and the queue is empty.
//
// rep receives the loops' reports; the MilestoneMsg goes to cfg.Send.
func drive(ctx context.Context, cfg Config, rep loop.Reporter, begin <-chan struct{}) (code int, finished bool) {
	control := cfg.First
	if cfg.Planning {
		select {
		case <-ctx.Done():
			return loop.ExitInterrupted, false
		case <-begin:
		}
		next, ok, err := nextQueued(ctx, cfg, rep)
		if err != nil || !ok {
			return loop.ExitInterrupted, false // only on Ctrl+C or /quit
		}
		control = next
	}
	for {
		l := cfg.NewLoop(control)
		l.Report, l.Controls = rep, cfg.Controls
		cfg.Send(MilestoneMsg{Milestone: l.Milestone, Control: l.Control})
		code = l.Run(ctx)
		if code != loop.ExitOK || !l.Finished() {
			return code, false // Ctrl+C, /quit or --once: the queue waits
		}
		if err := cfg.Queue.Remove(control.Number); err != nil {
			rep.Event(huddleEvent(fmt.Sprintf("warning: %v", err)))
		}
		next, ok, err := nextQueued(ctx, cfg, rep)
		if err != nil {
			return loop.ExitInterrupted, false
		}
		if !ok {
			return code, true
		}
		rep.Event(huddleEvent(fmt.Sprintf("Next in the queue: %s, control #%d", next.Milestone, next.Number)))
		control = next
	}
}

// nextQueued returns the next queued milestone's control issue, with
// state:dev-ready set if it had no label. A GitHub error is reported and
// retried every loop.PollInterval; err is set only when ctx ends.
func nextQueued(ctx context.Context, cfg Config, rep loop.Reporter) (gh.Issue, bool, error) {
	for {
		is, ok, err := cfg.Queue.Next(ctx, cfg.Control.Client, cfg.Repo)
		if err == nil {
			return is, ok, nil
		}
		rep.Event(huddleEvent(fmt.Sprintf("warning: read the milestone queue: %v; retrying in %s", err, loop.PollInterval)))
		select {
		case <-ctx.Done():
			return gh.Issue{}, false, ctx.Err()
		case <-time.After(loop.PollInterval):
		}
	}
}

// huddleEvent is an Activity event from Huddle itself, dated now.
func huddleEvent(text string) activity.Event {
	return activity.Event{Time: time.Now(), Actor: activity.ActorHuddle, Text: text}
}
