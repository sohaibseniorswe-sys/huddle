// Package state resolves Huddle's local state root once at startup.
package state

import (
	"fmt"
	"path/filepath"
)

// Resolve gives --home precedence over HUDDLE_HOME, then defaults to HOME/.huddle.
func Resolve(flag string, lookup func(string) (string, bool)) (string, error) {
	root := flag
	if root == "" {
		root, _ = lookup("HUDDLE_HOME")
	}
	if root == "" {
		home, _ := lookup("HOME")
		if home == "" {
			return "", fmt.Errorf("HOME is not set; use --home or HUDDLE_HOME")
		}
		root = filepath.Join(home, ".huddle")
	}
	return filepath.Abs(root)
}
