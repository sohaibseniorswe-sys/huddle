// Package lanes keeps the single build loop separate from unrestricted planning.
// Locks are scoped to the selected state root, including across Huddle processes.
package lanes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sohaibseniorswe-sys/huddle/internal/filelock"
)

// Build identifies a live build and its currently running agent CLI.
type Build struct {
	Project string `json:"project"`
	CLI     string `json:"cli"`
	PID     int    `json:"pid"`
}

// Lease is held for the entire build loop, including pauses and usage waits.
type Lease struct {
	root  string
	lock  *filelock.Lock
	build Build
}

// Acquire refuses a second build. Planning never calls Acquire.
func Acquire(root, project string) (*Lease, error) {
	l, err := filelock.Try(filepath.Join(root, "build.lock"))
	if errors.Is(err, filelock.ErrBusy) {
		return nil, fmt.Errorf("another build loop is active; stop it before starting project %s (planning remains available)", project)
	}
	if err != nil {
		return nil, err
	}
	lease := &Lease{root: root, lock: l, build: Build{Project: project, PID: os.Getpid()}}
	if err := lease.SetCLI(""); err != nil {
		l.Close()
		return nil, err
	}
	return lease, nil
}

// SetCLI updates usage metadata without changing ownership of the build lane.
func (l *Lease) SetCLI(cli string) error {
	l.build.CLI = cli
	b, err := json.Marshal(l.build)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(l.root, ".build-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(l.root, "build.json"))
}

// Close releases the lane. Stale metadata is harmless: Active checks the lock.
func (l *Lease) Close() error { return l.lock.Close() }

// Active reads metadata only while another process owns the lane.
func Active(root string) (Build, bool, error) {
	l, err := filelock.Try(filepath.Join(root, "build.lock"))
	if err == nil {
		return Build{}, false, l.Close()
	}
	if !errors.Is(err, filelock.ErrBusy) {
		return Build{}, false, err
	}
	b, err := os.ReadFile(filepath.Join(root, "build.json"))
	if err != nil {
		return Build{}, true, err
	}
	var build Build
	err = json.Unmarshal(b, &build)
	return build, true, err
}

// Warning describes potential shared subscription usage, never a planning block.
func Warning(companion string, b Build, active bool) string {
	if active && companion != "" && companion == b.CLI {
		return fmt.Sprintf("Usage contention: planning companion and live build %s both use %s; planning remains available", b.Project, companion)
	}
	return ""
}
