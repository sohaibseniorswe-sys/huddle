package agents

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeCLI writes a shell script that plays a CLI for Validate. The script
// records its arguments in args.txt, writes lastMessage to the file named
// after --output-last-message (as codex does), prints stderr and stdout,
// sleeps for sleep seconds and exits with code. The texts are kept in files
// next to the script, so no quoting can break them. It returns cli with
// Binary pointing at the script, and the path of args.txt.
func fakeCLI(t *testing.T, cli CLI, stdout, stderr, lastMessage string, code, sleep int) (CLI, string) {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"stdout.txt": stdout, "stderr.txt": stderr, "last.txt": lastMessage} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
d='` + dir + `'
printf '%s\n' "$@" > "$d/args.txt"
while [ $# -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then cp "$d/last.txt" "$2"; fi
  shift
done
cat > /dev/null
cat "$d/stderr.txt" >&2
cat "$d/stdout.txt"
sleep ` + strconv.Itoa(sleep) + `
exit ` + strconv.Itoa(code) + "\n"
	path := filepath.Join(dir, cli.Binary)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cli.Binary = path
	return cli, filepath.Join(dir, "args.txt")
}

// Recorded output of the real CLIs (Claude Code 2.1.299, codex-cli
// 0.160.0), shortened to the parts Huddle reads.
const (
	claudeOK         = `{"type":"result","subtype":"success","is_error":false,"session_id":"2ff1","result":"OK","modelUsage":{"claude-opus-5-5":{"outputTokens":4}}}` + "\n"
	claudeBadModel   = `{"type":"result","subtype":"success","is_error":true,"api_error_status":404,"session_id":"84f0","result":"There's an issue with the selected model (opux). It may not exist or you may not have access to it. Run --model to pick a different model."}` + "\n"
	claudeNoLogin    = `{"type":"result","subtype":"success","is_error":true,"session_id":"eb89","result":"Not logged in · Please run /login"}` + "\n"
	claudeEffortWarn = "Warning: Unknown --effort value 'max' — ignoring it and using the default effort. Valid values: low, medium, high, xhigh.\n"
	codexHeader      = "OpenAI Codex v0.160.0\n--------\nmodel: gpt-6.1-sol\nsession id: 01a13bd6-ec1a-7580-8fef-eaf9c06e5f4e\n--------\nuser\nReply with the word OK\n"
	codexBadModel    = codexHeader + "warning: Model metadata for `gpt-nope` not found.\n" +
		`ERROR: {"type":"error","status":400,"error":{"type":"invalid_request_error","message":"The 'gpt-nope' model is not supported when using Codex with a ChatGPT account."}}` + "\n"
	codexBadEffort = codexHeader +
		`ERROR: {"type":"error","error":{"message":"Invalid response.create payload: Invalid value: 'xhigh'. Supported values are: 'none', 'low'.","type":"invalid_request_error"},"status":400}` + "\n"
	codexNoLogin = codexHeader + "ERROR: Reconnecting... 5/5\n" +
		"ERROR: unexpected status 401 Unauthorized: Missing bearer or basic authentication in header, url: https://api.openai.com/v1/responses\n"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name           string
		cli            CLI
		model, effort  string
		stdout, stderr string
		last           string // what codex writes to --output-last-message
		code           int
		wantErr        string // "" means success
		wantArgs       []string
	}{
		{"claude ok", Claude(), "opus", "high", claudeOK, "", "", 0, "",
			[]string{"-p", "--model", "opus", "--effort", "high"}},
		{"claude default model and effort", Claude(), "", "", claudeOK, "", "", 0, "", []string{"-p"}},
		{"claude unknown model", Claude(), "opux", "", claudeBadModel, "", "", 1,
			`Claude Code rejected model "opux": There's an issue with the selected model (opux).`, nil},
		{"claude bad effort", Claude(), "haiku", "max", claudeOK, claudeEffortWarn, "", 0,
			"effort max is not available for Claude Code: Warning: Unknown --effort value 'max'", nil},
		{"claude not signed in", Claude(), "", "", claudeNoLogin, "", "", 1,
			`Claude Code is not signed in (run "claude auth login"): Not logged in`, nil},
		{"claude wrong answer", Claude(), "", "", `{"type":"result","is_error":false,"session_id":"x","result":"Hello there"}` + "\n", "", "", 0,
			`Claude Code answered "Hello there" instead of OK`, nil},
		{"codex ok", Codex(), "gpt-6.1-sol", "xhigh", "OK\n", codexHeader, "OK", 0, "",
			[]string{"exec", "--skip-git-repo-check", "-m", "gpt-6.1-sol", `model_reasoning_effort="xhigh"`, `sandbox_mode="read-only"`}},
		{"codex unknown model", Codex(), "gpt-nope", "", "", codexBadModel, "", 1,
			`Codex rejected model "gpt-nope": The 'gpt-nope' model is not supported when using Codex with a ChatGPT account.`, nil},
		{"codex bad effort", Codex(), "gpt-6.1-sol", "xhigh", "", codexBadEffort, "", 1,
			"effort xhigh is not available for this model: Invalid response.create payload: Invalid value: 'xhigh'.", nil},
		{"codex not signed in", Codex(), "", "", "", codexNoLogin, "", 1,
			`Codex is not signed in (run "codex login"): unexpected status 401 Unauthorized`, nil},
		{"effort not in the CLI's list", Claude(), "", "none", "", "", "", 0,
			"effort none is not one of Claude Code's: low, medium, high, xhigh, max", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli, argsFile := fakeCLI(t, tt.cli, tt.stdout, tt.stderr, tt.last, tt.code, 0)
			err := validate(context.Background(), Choice{CLI: cli, Model: tt.model, Effort: tt.effort}, 30*time.Second)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("Validate() error = %v, want nil", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("Validate() error = nil, want %q", tt.wantErr)
			case tt.wantErr != "" && !strings.HasPrefix(err.Error(), tt.wantErr):
				t.Fatalf("Validate() error = %q\nwant it to start with %q", err, tt.wantErr)
			}
			if tt.wantArgs == nil {
				return
			}
			data, err := os.ReadFile(argsFile)
			if err != nil {
				t.Fatal(err)
			}
			args := strings.Split(strings.TrimSpace(string(data)), "\n")
			for _, want := range tt.wantArgs {
				if !slices.Contains(args, want) {
					t.Errorf("arguments %q lack %q", args, want)
				}
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	cli, _ := fakeCLI(t, Claude(), claudeOK, "", "", 0, 9)
	start := time.Now()
	err := validate(context.Background(), Choice{CLI: cli}, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "Claude Code did not answer within 300ms") {
		t.Errorf("Validate() error = %v, want a timeout", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("Validate() took %s; the fake CLI was not killed", d)
	}
}

func TestValidateMissingBinary(t *testing.T) {
	cli := Codex()
	cli.Binary = filepath.Join(t.TempDir(), "no-such-codex")
	err := validate(context.Background(), Choice{CLI: cli}, time.Second)
	if err == nil || !strings.HasPrefix(err.Error(), "Codex could not start") {
		t.Errorf("Validate() error = %v, want could not start", err)
	}
}
