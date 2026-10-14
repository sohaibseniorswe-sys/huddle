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

// Commit stages files (paths relative to dir) and commits only them with
// message, leaving anything else staged or changed as it was.
func Commit(ctx context.Context, dir, message string, files ...string) error {
	args := append([]string{"add", "--"}, files...)
	if _, err := run(ctx, dir, args...); err != nil {
		return err
	}
	args = append([]string{"commit", "-m", message, "--"}, files...)
	_, err := run(ctx, dir, args...)
	return err
}
