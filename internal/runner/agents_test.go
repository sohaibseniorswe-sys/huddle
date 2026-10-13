package runner

import (
	"os"
	"strings"
	"testing"
)

func TestAgentCommands(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{"dev", Dev("/repo", "do it", []string{"/gomod", "/gocache"}), `codex exec --strict-config --ignore-user-config ` +
			`-c sandbox_mode="workspace-write" -c approval_policy="never" -c sandbox_workspace_write.network_access=true ` +
			`--add-dir /gomod --add-dir /gocache -`},
		{"pm", PM("/repo", "review it"), `claude -p --output-format json --permission-mode dontAsk --allowedTools ` +
			`Read Edit(/Session.md) Bash(git log *) Bash(git diff *) Bash(git status *) Bash(git show *) ` +
			`Bash(gh issue view *) Bash(gh issue list *) Bash(gh pr view *) Bash(gh pr list *) Bash(gh pr diff *) Bash(gh pr checks *) ` +
			`--disallowedTools Bash(git push *) Bash(gh pr merge *) Bash(gh issue edit *) Bash(gh label create *)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(append([]string{tt.spec.Name}, tt.spec.Args...), " ")
			if got != tt.want {
				t.Errorf("command = %s\nwant      %s", got, tt.want)
			}
			if tt.spec.Dir != "/repo" {
				t.Errorf("Dir = %q, want /repo", tt.spec.Dir)
			}
			if tt.spec.Stdin == "" {
				t.Error("prompt must go on stdin")
			}
			for _, a := range tt.spec.Args {
				if a == tt.spec.Stdin {
					t.Error("prompt must not be a command-line argument")
				}
			}
		})
	}
}

func TestGoCacheDirs(t *testing.T) {
	mod, cache := t.TempDir()+"/mod", t.TempDir()+"/build"
	t.Setenv("GOMODCACHE", mod)
	t.Setenv("GOCACHE", cache)
	dirs, err := GoCacheDirs(t.TempDir())
	if err != nil {
		t.Fatalf("GoCacheDirs() error = %v", err)
	}
	if strings.Join(dirs, ",") != mod+","+cache {
		t.Errorf("GoCacheDirs() = %v, want [%s %s]", dirs, mod, cache)
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("cache dir not created: %v", err)
		}
	}
}
