package loop

import (
	"context"
	"sync"
)

// Controls lets the run screen steer the loop while the loop runs in its
// own goroutine: pause it between turns, resume it, and wake it early from
// a poll wait (for example right after /reply changed the label, instead of
// up to 30 seconds later).
//
// The run screen and the loop use Controls from different goroutines, so
// every field is guarded by a mutex. The zero value is ready to use and
// not paused.
type Controls struct {
	mu     sync.Mutex
	paused bool
	// wake ends the loop's current nap early; nil while the loop is not
	// napping.
	wake context.CancelFunc
	// woken records a Wake that came while the loop was not napping, so the
	// next nap ends straight away instead of the wake being lost.
	woken bool
}

// Pause asks the loop not to start another turn. A running turn finishes
// normally; Pause never stops an agent.
func (c *Controls) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = true
}

// Resume lets the loop start turns again, straight away.
func (c *Controls) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = false
	c.wakeLocked()
}

// Paused reports whether the loop is paused.
func (c *Controls) Paused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

// Wake makes the loop read the control issue's label now instead of at the
// end of its current poll wait.
func (c *Controls) Wake() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wakeLocked()
}

// wakeLocked wakes the loop; c.mu must be held.
func (c *Controls) wakeLocked() {
	if c.wake != nil {
		c.wake()
		c.wake = nil
		return
	}
	c.woken = true
}

// napContext returns a context for one poll wait: it is canceled when
// parent is (Ctrl+C) or when Wake or Resume is called. The caller must call
// the returned cancel function when the wait is over.
//
// A context.Context is Go's standard way to tell a blocking call to stop
// early: Clock.Sleep returns as soon as the context is canceled.
func (c *Controls) napContext(parent context.Context) (context.Context, context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithCancel(parent)
	if c.woken {
		c.woken = false
		cancel() // a Wake already came: do not wait at all
		return ctx, cancel
	}
	c.wake = cancel
	return ctx, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.wake = nil
		cancel()
	}
}
