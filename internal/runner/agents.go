package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// The agent commands are hardcoded (section 6 of the plan). Flags were checked
// against codex-cli 0.160.0 and Claude Code 2.1.296; see README.md.
//
// Both prompts go on stdin: it avoids argument length limits, and for claude
// it avoids the variadic --allowedTools/--disallowedTools swallowing a
// trailing prompt argument.

// DevConfig are the codex config overrides for a Dev turn. They are passed
// with -c so the command does not depend on ~/.codex/config.toml, which
// --ignore-user-config skips entirely. --strict-config makes codex reject a
// misspelt key or value instead of silently ignoring it.
var DevConfig = []string{
	`sandbox_mode="workspace-write"`,
	`approval_policy="never"`,
	`sandbox_workspace_write.network_access=true`,
}

// PMAllowedTools are the only tools the PM may use beyond those that never
// need approval. Under dontAsk anything else is denied. The Edit rule covers
// every file-editing tool (Write included); "/" anchors it at the working
// directory, so only the repo root's Session.md matches.
var PMAllowedTools = []string{
	"Read",
	"Edit(/Session.md)",
	"Bash(git log *)",
	"Bash(git diff *)",
	"Bash(git status *)",
	"Bash(git show *)",
	"Bash(gh issue view *)",
	"Bash(gh issue list *)",
	"Bash(gh pr view *)",
	"Bash(gh pr list *)",
	"Bash(gh pr diff *)",
	"Bash(gh pr checks *)",
}

// PMDisallowedTools are denied explicitly. dontAsk already denies them, but
// a deny rule also beats any allow rule from the target repo's settings.
var PMDisallowedTools = []string{
	"Bash(git push *)",
	"Bash(gh pr merge *)",
	"Bash(gh issue edit *)",
	"Bash(gh label create *)",
}

// Dev returns the Spec for a Codex turn in dir. writable are extra
// directories the sandbox may write to, normally GoCacheDirs. "-" reads the
// prompt from stdin.
func Dev(dir, prompt string, writable []string) Spec {
	args := []string{"exec", "--strict-config", "--ignore-user-config"}
	for _, kv := range DevConfig {
		args = append(args, "-c", kv)
	}
	for _, d := range writable {
		args = append(args, "--add-dir", d)
	}
	return Spec{
		Name:  "codex",
		Args:  append(args, "-"),
		Stdin: prompt,
		Dir:   dir,
	}
}

// GoCacheDirs returns Go's module and build caches as seen from dir, creating
// them if missing, so the Dev sandbox can make them writable for go build
// and go test.
func GoCacheDirs(dir string) ([]string, error) {
	cmd := exec.Command("go", "env", "GOMODCACHE", "GOCACHE")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go env GOMODCACHE GOCACHE: %w", err)
	}
	var dirs []string
	for _, d := range strings.Fields(string(out)) {
		if d == "off" { // GOCACHE=off
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create Go cache dir: %w", err)
		}
		dirs = append(dirs, d)
	}
	return dirs, nil
}

// PM returns the Spec for a Claude Code turn in dir. Its stdout is a JSON
// envelope whose "result" field holds the answer text.
func PM(dir, prompt string) Spec {
	args := []string{"-p", "--output-format", "json", "--permission-mode", "dontAsk", "--allowedTools"}
	args = append(args, PMAllowedTools...)
	args = append(args, "--disallowedTools")
	return Spec{
		Name:  "claude",
		Args:  append(args, PMDisallowedTools...),
		Stdin: prompt,
		Dir:   dir,
	}
}
