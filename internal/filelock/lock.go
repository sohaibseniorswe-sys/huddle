// Package filelock supplies process-scoped OS locks. The kernel releases a lock
// on crash; lock files are retained so concurrent opens always use one inode.
package filelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrBusy means another process owns the lock.
var ErrBusy = errors.New("lock is busy")

// Lock owns an open file until Close. Never copy a Lock.
type Lock struct{ file *os.File }

// Try attempts a nonblocking exclusive lock.
func Try(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Wait acquires a store lock, honoring cancellation between short retries.
func Wait(ctx context.Context, path string) (*Lock, error) {
	for {
		l, err := Try(path)
		if !errors.Is(err, ErrBusy) {
			return l, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// Close releases the lock and closes its handle.
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
