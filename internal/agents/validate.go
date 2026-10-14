package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
)

// ValidateTimeout is how long Validate waits for the CLI's answer.
const ValidateTimeout = 2 * time.Minute

// validatePrompt is the tiny prompt Validate sends.
const validatePrompt = "Reply with the word OK"

// Validate checks that cli can run with model and effort by sending a
// tiny prompt ("Reply with the word OK") and checking the answer. An empty
// model or effort means the CLI's default.
//
// The error is readable as it is, for the setup wizard to show: for example
// `Claude Code rejected model "opux": There's an issue with the selected
// model (opux)...`, `effort xhigh is not available for this model: ...`,
// `Codex is not signed in (run "codex login")`, or the CLI's own message.
//
// The run uses the subscription login: runner.Run removes the API key
// variables from the environment, as it does for every agent. It runs in
// an empty temporary folder with no tools that change anything, and gives
// up after ValidateTimeout.
func Validate(ctx context.Context, cli CLI, model, effort string) error {
	return validate(ctx, Choice{CLI: cli, Model: model, Effort: effort}, ValidateTimeout)
}

// validate is Validate with the time limit as a parameter, so tests can
// use a short one.
func validate(ctx context.Context, c Choice, timeout time.Duration) error {
	if !c.CLI.ValidEffort(c.Effort) {
		return fmt.Errorf("effort %s is not one of %s's: %s", c.Effort, c.CLI.Name, strings.Join(c.CLI.Efforts, ", "))
	}
	dir, err := os.MkdirTemp("", "huddle-validate-")
	if err != nil {
		return fmt.Errorf("create a folder for the test run: %w", err)
	}
	defer os.RemoveAll(dir)

	last := filepath.Join(dir, "last-message.txt")
	spec := runner.Spec{
		Name:    c.CLI.Binary,
		Args:    validateArgs(c, last),
		Stdin:   validatePrompt,
		Dir:     dir,
		Timeout: timeout,
	}
	res, err := runner.Run(ctx, spec)
	if err != nil {
		return fmt.Errorf("%s could not start: %w", c.CLI.Name, err)
	}
	return checkValidation(c, res, last, timeout)
}

// validateArgs are the arguments of the test run: like the companion's
// command, nothing it runs can change anything, but it gets no tools at
// all. Codex is told to run outside a git repository, which it otherwise
// refuses.
func validateArgs(c Choice, lastMessage string) []string {
	if c.CLI.ID == ClaudeID {
		args := []string{"-p", "--output-format", "json", "--tools", "", "--strict-mcp-config", "--setting-sources", "user"}
		return append(args, claudeModelArgs(c)...)
	}
	args := []string{"exec", "--strict-config", "--ignore-user-config", "--skip-git-repo-check"}
	for _, kv := range codexReadOnly {
		args = append(args, "-c", kv)
	}
	args = append(args, codexModelArgs(c)...)
	return append(args, "--output-last-message", lastMessage, "-")
}

// checkValidation turns the test run's result into Validate's error.
func checkValidation(c Choice, res runner.Result, lastMessage string, timeout time.Duration) error {
	name := c.CLI.Name
	if res.TimedOut {
		return fmt.Errorf("%s did not answer within %s", name, timeout)
	}
	if res.Canceled {
		return errors.New("the test run was stopped")
	}
	all := res.Head + "\n" + res.Tail
	// Claude Code does not fail on an effort it does not know: it warns
	// on stderr and uses its default, which is not what was asked for.
	if line := lineWith(all, "Unknown --effort value"); line != "" {
		return fmt.Errorf("effort %s is not available for %s: %s", c.Effort, name, line)
	}
	if u := Usage(c.CLI, all+"\n"+res.Stdout, time.Now()); u.Hit || u.Escalate {
		return fmt.Errorf("%s hit a usage limit: %s", name, u.Message)
	}

	out := Read(c.CLI, res, lastMessage)
	if !out.Failed {
		if !strings.Contains(strings.ToUpper(out.Message), "OK") {
			return fmt.Errorf("%s answered %q instead of OK", name, firstLineOf(out.Message))
		}
		return nil
	}

	msg := failureMessage(c.CLI, res, out)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not logged in") || strings.Contains(lower, "401 unauthorized") ||
		strings.Contains(lower, "please run /login") || strings.Contains(lower, "missing bearer"):
		return fmt.Errorf("%s is not signed in (run %q): %s", name, loginCommand(c.CLI), msg)
	case c.Effort != "" && (strings.Contains(lower, "effort") || strings.Contains(lower, "reasoning") ||
		strings.Contains(lower, "'"+strings.ToLower(c.Effort)+"'")):
		return fmt.Errorf("effort %s is not available for this model: %s", c.Effort, msg)
	case strings.Contains(lower, "model"):
		model := c.Model
		if model == "" {
			model = "(its default)"
		}
		return fmt.Errorf("%s rejected model %q: %s", name, model, msg)
	}
	return fmt.Errorf("%s failed: %s", name, msg)
}

// failureMessage is the CLI's own message for a failed run: Claude Code's
// envelope result, or the last "ERROR:" line Codex printed (with the
// message taken out of its JSON when it has some), or else the last line.
func failureMessage(cli CLI, res runner.Result, out Output) string {
	if cli.ID == ClaudeID {
		if env, err := ParseClaude(res.Stdout); err == nil && strings.TrimSpace(env.Result) != "" {
			return firstLineOf(env.Result)
		}
		return lastLineOf(res.Tail)
	}
	lines := strings.Split(res.Tail, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		text, ok := strings.CutPrefix(strings.TrimSpace(lines[i]), "ERROR: ")
		if !ok {
			continue
		}
		// The API's errors arrive as JSON: {"error": {"message": "..."}}.
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(text), &apiErr) == nil && apiErr.Error.Message != "" {
			return apiErr.Error.Message
		}
		return text
	}
	if msg := strings.TrimSpace(out.Message); msg != "" {
		return lastLineOf(msg)
	}
	return fmt.Sprintf("exit code %d", res.ExitCode)
}

// loginCommand is how the user signs cli in.
func loginCommand(cli CLI) string {
	if cli.ID == ClaudeID {
		return "claude auth login"
	}
	return "codex login"
}

// lineWith returns the first line of s that contains sub, trimmed, or "".
func lineWith(s, sub string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, sub) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// firstLineOf returns the first line of s, trimmed.
func firstLineOf(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// lastLineOf returns the last non-empty line of s, trimmed.
func lastLineOf(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
