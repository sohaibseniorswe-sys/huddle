package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const remoteFixtureRepo = "sohaibseniorswe-sys/palantir"

func TestValidateRepo(t *testing.T) {
	valid := []string{
		"sohaibseniorswe-sys/palantir",
		"sohaibseniorswe-sys/huddle-sandbox",
		"a/b",
		"Owner/repo.name_1",
	}
	invalid := []string{
		"",
		"palantir",
		"owner/",
		"/repo",
		"owner/repo/extra",
		"owner repo/x",
		"https://github.com/owner/repo",
		"owner/..",
		"-owner/repo",
	}
	for _, s := range valid {
		if err := ValidateRepo(s); err != nil {
			t.Errorf("ValidateRepo(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range invalid {
		if err := ValidateRepo(s); err == nil {
			t.Errorf("ValidateRepo(%q) = nil, want error", s)
		}
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"https://github.com/sohaibseniorswe-sys/palantir", "sohaibseniorswe-sys/palantir"},
		{"https://github.com/sohaibseniorswe-sys/palantir/", "sohaibseniorswe-sys/palantir"},
		{"http://github.com/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"https://user:token@github.com/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"git@github.com:sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"git@github.com:sohaibseniorswe-sys/palantir", "sohaibseniorswe-sys/palantir"},
		{"ssh://git@github.com/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"ssh://git@github.com/sohaibseniorswe-sys/palantir", "sohaibseniorswe-sys/palantir"},
		{"ssh://git@ssh.github.com:443/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"git://github.com/sohaibseniorswe-sys/palantir.git", "sohaibseniorswe-sys/palantir"},
		{"  https://GitHub.com/sohaibseniorswe-sys/huddle-sandbox.git\n", "sohaibseniorswe-sys/huddle-sandbox"},
	}
	for _, tt := range tests {
		got, err := ParseRemoteURL(tt.url)
		if err != nil {
			t.Errorf("ParseRemoteURL(%q) error: %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseRemoteURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestParseRemoteURLInvalid(t *testing.T) {
	for _, u := range []string{
		"",
		"/home/sweeng/projects/palantir",
		"https://gitlab.com/sohaibseniorswe-sys/palantir.git",
		"git@gitlab.com:sohaibseniorswe-sys/palantir.git",
		"https://github.com/sohaibseniorswe-sys",
		"https://github.com/a/b/c",
		"ftp://github.com/a/b",
	} {
		if got, err := ParseRemoteURL(u); err == nil {
			t.Errorf("ParseRemoteURL(%q) = %q, want error", u, got)
		}
	}
}

func TestMatchRemote(t *testing.T) {
	const sandbox = "sohaibseniorswe-sys/huddle-sandbox"
	tests := []struct {
		name   string
		remote string
		target string
		ok     bool
	}{
		{"ordinary target https", "https://github.com/sohaibseniorswe-sys/palantir.git", remoteFixtureRepo, true},
		{"ordinary target ssh", "git@github.com:sohaibseniorswe-sys/palantir.git", remoteFixtureRepo, true},
		{"ordinary target case-insensitive", "https://github.com/SohaibSeniorSWE-sys/Palantir", remoteFixtureRepo, true},
		{"ordinary target, sandbox clone", "https://github.com/sohaibseniorswe-sys/huddle-sandbox.git", remoteFixtureRepo, false},
		{"other target https", "https://github.com/sohaibseniorswe-sys/huddle-sandbox.git", sandbox, true},
		{"other target ssh", "ssh://git@github.com/sohaibseniorswe-sys/huddle-sandbox", sandbox, true},
		{"other target, palantir clone", "git@github.com:sohaibseniorswe-sys/palantir.git", sandbox, false},
		{"prefix is not a match", "https://github.com/sohaibseniorswe-sys/palantir-old.git", remoteFixtureRepo, false},
		{"unparseable remote", "not a url", remoteFixtureRepo, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MatchRemote(tt.remote, tt.target)
			if (err == nil) != tt.ok {
				t.Errorf("MatchRemote(%q, %q) = %v, want ok=%v", tt.remote, tt.target, err, tt.ok)
			}
		})
	}
}

func TestEnsureAgentFiles(t *testing.T) {
	write := func(t *testing.T, dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("missing files", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "CLAUDE.md", "x")
		_, err := EnsureAgentFiles(dir)
		if err == nil || !strings.Contains(err.Error(), "AGENTS.md") {
			t.Fatalf("EnsureAgentFiles() = %v, want error mentioning AGENTS.md", err)
		}
		if _, err := os.Stat(filepath.Join(dir, SessionFile)); err == nil {
			t.Error("Session.md created although checks failed")
		}
	})

	t.Run("creates Session.md", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "AGENTS.md", "x")
		write(t, dir, "CLAUDE.md", "x")
		created, err := EnsureAgentFiles(dir)
		if err != nil || !created {
			t.Fatalf("EnsureAgentFiles() = %v, %v; want true, nil", created, err)
		}
		if b, err := os.ReadFile(filepath.Join(dir, SessionFile)); err != nil || len(b) != 0 {
			t.Fatalf("Session.md = %q, %v; want empty file", b, err)
		}
	})

	t.Run("keeps existing Session.md", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "AGENTS.md", "x")
		write(t, dir, "CLAUDE.md", "x")
		write(t, dir, SessionFile, "notes")
		created, err := EnsureAgentFiles(dir)
		if err != nil || created {
			t.Fatalf("EnsureAgentFiles() = %v, %v; want false, nil", created, err)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, SessionFile)); string(b) != "notes" {
			t.Errorf("Session.md overwritten: %q", b)
		}
	})
}
