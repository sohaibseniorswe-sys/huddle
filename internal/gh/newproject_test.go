package gh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateRepositoryCreationAndRecovery(t *testing.T) {
	for _, mode := range []string{"new", "owned", "unowned", "public", "forbidden"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			script := `#!/bin/sh
printf '%s\n' "$*" >> "$NEW_REPO_FIXTURE/log"
case "$*" in
 'api -X GET repos/owner/new')
 case "$NEW_REPO_MODE" in
 forbidden) echo 'gh: forbidden (HTTP 403)' >&2; exit 1;;
 new) if ! test -f "$NEW_REPO_FIXTURE/created"; then echo 'gh: Not Found (HTTP 404)' >&2; exit 1; fi;;
 esac
 case "$NEW_REPO_MODE" in
 unowned) echo '{"private":true,"description":"Someone else","default_branch":"main"}';;
 public) echo '{"private":false,"description":"Huddle new project id","default_branch":"main"}';;
 *) echo '{"private":true,"description":"Huddle new project id","default_branch":"main"}';;
 esac;;
 'repo create owner/new --private --add-readme --description Huddle new project id') touch "$NEW_REPO_FIXTURE/created";;
 'api -X GET repos/owner/new/git/ref/heads/v4') echo 'gh: Not Found (HTTP 404)' >&2; exit 1;;
 'api -X GET repos/owner/new/git/ref/heads/main') echo '{"object":{"sha":"initial-commit"}}';;
 'api -X POST repos/owner/new/git/refs --input -') cat > "$NEW_REPO_FIXTURE/ref"; echo '{}';;
 *) echo "unexpected args: $*" >&2; exit 1;;
esac
`
			if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("NEW_REPO_FIXTURE", dir)
			t.Setenv("NEW_REPO_MODE", mode)
			err := (CLI{}).EnsurePrivateRepository(context.Background(), "owner/new", "Huddle new project id", "v4")
			log, _ := os.ReadFile(filepath.Join(dir, "log"))
			switch mode {
			case "new", "owned":
				if err != nil {
					t.Fatal(err)
				}
				ref, _ := os.ReadFile(filepath.Join(dir, "ref"))
				if !strings.Contains(string(ref), "refs/heads/v4") || !strings.Contains(string(ref), "initial-commit") {
					t.Fatalf("bad branch creation %s", ref)
				}
				if mode == "owned" && strings.Contains(string(log), "repo create") {
					t.Fatal("retried repo creation instead of resuming")
				}
			default:
				if err == nil {
					t.Fatal("accepted foreign/public/unavailable repository")
				}
				if strings.Contains(string(log), "repo create") || strings.Contains(string(log), "POST") {
					t.Fatalf("mutated refused repo: %s", log)
				}
			}
		})
	}
}
