// Package gitx holds small git helpers. It shells out to the git binary.
package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// run runs git in dir and returns its trimmed stdout.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// TopLevel returns the root of the git work tree containing dir.
func TopLevel(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "rev-parse", "--show-toplevel")
}

// RemoteURL returns the URL of the named remote.
func RemoteURL(ctx context.Context, dir, remote string) (string, error) {
	return run(ctx, dir, "remote", "get-url", remote)
}

// StatusPorcelain returns the raw output of `git status --porcelain -z`.
func StatusPorcelain(ctx context.Context, dir string) (string, error) {
	return run(ctx, dir, "status", "--porcelain", "-z")
}

// DirtyPaths parses `git status --porcelain -z` output and returns the paths
// that are modified, staged or untracked, except those listed in ignore.
// For renames and copies only the new path is reported.
func DirtyPaths(porcelainZ string, ignore ...string) []string {
	skip := make(map[string]bool, len(ignore))
	for _, p := range ignore {
		skip[p] = true
	}
	var dirty []string
	fields := strings.Split(porcelainZ, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		xy, path := entry[:2], entry[3:]
		// Renames and copies are followed by a field holding the original path.
		if xy[0] == 'R' || xy[0] == 'C' || xy[1] == 'R' || xy[1] == 'C' {
			i++
		}
		if !skip[path] {
			dirty = append(dirty, path)
		}
	}
	return dirty
}
