// Package checks implements Huddle's startup checks (MVP_PLAN.md section 7).
package checks

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultRepo is the target repo when --repo is not given.
const DefaultRepo = "sohaibseniorswe-sys/palantir"

// SessionFile is the shared, gitignored scratchpad both agents read and update.
const SessionFile = "Session.md"

var repoRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)

// ValidateRepo checks that s has the form "owner/name".
func ValidateRepo(s string) error {
	if !repoRe.MatchString(s) {
		return fmt.Errorf("invalid repo %q: want owner/name", s)
	}
	name := s[strings.Index(s, "/")+1:]
	if name == "." || name == ".." {
		return fmt.Errorf("invalid repo %q: want owner/name", s)
	}
	return nil
}

// ParseRemoteURL extracts "owner/name" from a GitHub remote URL. It accepts
// HTTPS, SSH (scp-like and ssh://) and git:// forms, with or without ".git".
func ParseRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("parse remote URL %q: %w", raw, err)
		}
		switch u.Scheme {
		case "https", "http", "ssh", "git", "git+ssh":
		default:
			return "", fmt.Errorf("unsupported remote URL scheme in %q", raw)
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like syntax: [user@]host:owner/name
		at := strings.LastIndex(raw, "@")
		colon := strings.Index(raw, ":")
		if colon < 0 || colon < at {
			return "", fmt.Errorf("unrecognised remote URL %q", raw)
		}
		host, path = raw[at+1:colon], raw[colon+1:]
	}
	if !strings.EqualFold(host, "github.com") && !strings.EqualFold(host, "ssh.github.com") {
		return "", fmt.Errorf("remote %q is not on github.com", raw)
	}
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if err := ValidateRepo(path); err != nil {
		return "", fmt.Errorf("remote %q does not point to an owner/name repo", raw)
	}
	return path, nil
}

// MatchRemote reports an error unless the remote URL points at target.
// GitHub owner and repo names are case-insensitive.
func MatchRemote(remoteURL, target string) error {
	got, err := ParseRemoteURL(remoteURL)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, target) {
		return fmt.Errorf("origin is %s, but the target repo is %s", got, target)
	}
	return nil
}

// EnsureAgentFiles checks that AGENTS.md and CLAUDE.md exist in root and
// creates an empty Session.md if it is missing. It reports whether
// Session.md was created.
func EnsureAgentFiles(root string) (createdSession bool, err error) {
	var missing []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				missing = append(missing, name)
				continue
			}
			return false, err
		}
	}
	if len(missing) > 0 {
		return false, fmt.Errorf("missing in %s: %s", root, strings.Join(missing, ", "))
	}
	f, err := os.OpenFile(filepath.Join(root, SessionFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, f.Close()
}
