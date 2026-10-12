package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// APIKeyVars are the environment variables that would let an agent CLI bill
// against an API account. Huddle refuses to start if either is set.
var APIKeyVars = []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}

// CheckAPIKeys fails if any of APIKeyVars is set, even to an empty value.
// lookup is normally os.LookupEnv.
func CheckAPIKeys(lookup func(string) (string, bool)) error {
	var set []string
	for _, v := range APIKeyVars {
		if _, ok := lookup(v); ok {
			set = append(set, v)
		}
	}
	if len(set) > 0 {
		return fmt.Errorf("refusing to start: %s set in the environment (Huddle must never use API billing; unset it and use a subscription login)",
			strings.Join(set, " and "))
	}
	return nil
}

// RunFunc runs a command and returns its stdout and stderr.
type RunFunc func(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)

// ExecRun is the real RunFunc.
func ExecRun(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// AgentCLIs checks that codex and claude are installed and logged in with a
// subscription rather than an API key.
type AgentCLIs struct {
	LookPath func(string) (string, error) // normally exec.LookPath
	Run      RunFunc                      // normally ExecRun
}

// Check runs all agent CLI checks.
func (a AgentCLIs) Check(ctx context.Context) error {
	for _, bin := range []string{"codex", "claude"} {
		if _, err := a.LookPath(bin); err != nil {
			return fmt.Errorf("%s not found on PATH", bin)
		}
	}

	stdout, stderr, err := a.Run(ctx, "codex", "login", "status")
	if err != nil {
		return fmt.Errorf("codex is not logged in (run `codex login`): %s", firstLine(stdout+stderr, err))
	}
	if err := CheckCodexLogin(stdout + stderr); err != nil {
		return err
	}

	stdout, stderr, err = a.Run(ctx, "claude", "auth", "status", "--json")
	if err != nil {
		return fmt.Errorf("claude is not logged in (run `claude auth login`): %s", firstLine(stdout+stderr, err))
	}
	return CheckClaudeLogin(stdout)
}

// CheckCodexLogin checks `codex login status` output for a ChatGPT login.
// Codex prints e.g. "Logged in using ChatGPT" (to stderr) for a subscription.
func CheckCodexLogin(out string) error {
	if strings.Contains(out, "Logged in using ChatGPT") {
		return nil
	}
	return fmt.Errorf("codex must be logged in with ChatGPT, not an API key (run `codex logout && codex login`); `codex login status` said: %s",
		firstLine(out, nil))
}

// CheckClaudeLogin checks `claude auth status --json` output for a claude.ai
// subscription login with Anthropic as the provider.
func CheckClaudeLogin(out string) error {
	var st struct {
		LoggedIn         bool   `json:"loggedIn"`
		AuthMethod       string `json:"authMethod"`
		APIProvider      string `json:"apiProvider"`
		SubscriptionType string `json:"subscriptionType"`
	}
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		return fmt.Errorf("cannot read `claude auth status --json` output: %w", err)
	}
	switch {
	case !st.LoggedIn:
		return fmt.Errorf("claude is not logged in (run `claude auth login`)")
	case st.AuthMethod != "claude.ai":
		return fmt.Errorf("claude must be logged in with a claude.ai subscription, but auth method is %q (run `claude auth login`)", st.AuthMethod)
	case st.APIProvider != "firstParty":
		return fmt.Errorf("claude must use Anthropic directly, but API provider is %q", st.APIProvider)
	}
	return nil
}

func firstLine(s string, err error) string {
	s = strings.TrimSpace(s)
	if s == "" {
		if err != nil {
			return err.Error()
		}
		return "(no output)"
	}
	line, _, _ := strings.Cut(s, "\n")
	return line
}
