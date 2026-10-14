package checks

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestCheckAPIKeys(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr []string
	}{
		{"none set", map[string]string{"HOME": "/home/x"}, nil},
		{"OpenAI key set", map[string]string{"OPENAI_API_KEY": "sk-x"}, []string{"OPENAI_API_KEY"}},
		{"Anthropic key set", map[string]string{"ANTHROPIC_API_KEY": "sk-ant-x"}, []string{"ANTHROPIC_API_KEY"}},
		{"both set", map[string]string{"OPENAI_API_KEY": "a", "ANTHROPIC_API_KEY": "b"}, []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"}},
		{"set but empty", map[string]string{"ANTHROPIC_API_KEY": ""}, []string{"ANTHROPIC_API_KEY"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(k string) (string, bool) { v, ok := tt.env[k]; return v, ok }
			err := CheckAPIKeys(lookup)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("CheckAPIKeys() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatal("CheckAPIKeys() = nil, want error")
			}
			for _, w := range tt.wantErr {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("CheckAPIKeys() = %v, want mention of %s", err, w)
				}
			}
		})
	}
}

// TestCheckAPIKeysRealEnv exercises the guard with the real environment.
func TestCheckAPIKeysRealEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	if err := CheckAPIKeys(os.LookupEnv); err == nil {
		t.Error("CheckAPIKeys() = nil with OPENAI_API_KEY set")
	}
}

const claudeSubscription = `{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty", "subscriptionType": "pro"}`

func TestCheckCodexLogin(t *testing.T) {
	if err := CheckCodexLogin("Logged in using ChatGPT\n"); err != nil {
		t.Errorf("ChatGPT login rejected: %v", err)
	}
	for _, out := range []string{"Logged in using an API key - sk-proj-***", "Not logged in", ""} {
		if err := CheckCodexLogin(out); err == nil {
			t.Errorf("CheckCodexLogin(%q) = nil, want error", out)
		}
	}
}

func TestCheckClaudeLogin(t *testing.T) {
	tests := []struct {
		name string
		out  string
		ok   bool
	}{
		{"subscription", claudeSubscription, true},
		{"not logged in", `{"loggedIn": false}`, false},
		{"api key", `{"loggedIn": true, "authMethod": "api_key", "apiProvider": "firstParty"}`, false},
		{"bedrock", `{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "bedrock"}`, false},
		{"garbage", `Login method: Claude Pro account`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckClaudeLogin(tt.out); (err == nil) != tt.ok {
				t.Errorf("CheckClaudeLogin() = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestAgentCLIsCheck(t *testing.T) {
	ctx := context.Background()
	allFound := func(string) (string, error) { return "/usr/bin/x", nil }
	runWith := func(codexErr string, claudeOut string) RunFunc {
		return func(_ context.Context, name string, args ...string) (string, string, error) {
			if len(args) == 1 && args[0] == "--version" {
				return map[string]string{"codex": "codex-cli 0.160.0\n", "claude": "2.1.299 (Claude Code)\n"}[name], "", nil
			}
			switch name {
			case "codex":
				if codexErr != "" {
					return "", codexErr, errors.New("exit status 1")
				}
				return "", "Logged in using ChatGPT\n", nil
			case "claude":
				return claudeOut, "", nil
			}
			return "", "", errors.New("unexpected command " + name)
		}
	}

	both := []string{"codex", "claude"}

	t.Run("all good", func(t *testing.T) {
		a := AgentCLIs{LookPath: allFound, Run: runWith("", claudeSubscription)}
		versions, err := a.Check(ctx, both)
		if err != nil {
			t.Errorf("Check() = %v", err)
		}
		if versions["codex"] != "0.160.0" || versions["claude"] != "2.1.299" {
			t.Errorf("versions = %v", versions)
		}
	})
	t.Run("claude missing", func(t *testing.T) {
		lp := func(b string) (string, error) {
			if b == "claude" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + b, nil
		}
		a := AgentCLIs{LookPath: lp, Run: runWith("", claudeSubscription)}
		if _, err := a.Check(ctx, both); err == nil || !strings.Contains(err.Error(), "claude not found") {
			t.Errorf("Check() = %v, want claude not found", err)
		}
	})
	t.Run("codex not logged in", func(t *testing.T) {
		a := AgentCLIs{LookPath: allFound, Run: runWith("Not logged in", claudeSubscription)}
		if _, err := a.Check(ctx, both); err == nil || !strings.Contains(err.Error(), "codex") {
			t.Errorf("Check() = %v, want codex error", err)
		}
	})
	t.Run("claude using api key", func(t *testing.T) {
		a := AgentCLIs{LookPath: allFound, Run: runWith("", `{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty"}`)}
		if _, err := a.Check(ctx, both); err == nil || !strings.Contains(err.Error(), "api_key") {
			t.Errorf("Check() = %v, want api_key error", err)
		}
	})
	t.Run("only the CLIs in use are checked", func(t *testing.T) {
		lp := func(b string) (string, error) {
			if b == "codex" {
				return "", errors.New("not found")
			}
			return "/usr/bin/" + b, nil
		}
		a := AgentCLIs{LookPath: lp, Run: runWith("Not logged in", claudeSubscription)}
		versions, err := a.Check(ctx, []string{"claude"})
		if err != nil {
			t.Errorf("Check(claude only) = %v; codex is not used, so it must not be checked", err)
		}
		if _, ok := versions["codex"]; ok {
			t.Errorf("versions = %v, want claude only", versions)
		}
	})
}
