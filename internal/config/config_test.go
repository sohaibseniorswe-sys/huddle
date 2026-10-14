package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
)

const repo = "sohaibseniorswe-sys/palantir"

// write puts text in repo's settings file under home.
func write(t *testing.T, home, text string) {
	t.Helper()
	path := Path(home, repo)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPath(t *testing.T) {
	if got, want := Path("/home/u", repo), "/home/u/.huddle/projects/sohaibseniorswe-sys_palantir.yaml"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoadDefaults(t *testing.T) {
	c, exists, err := Load(t.TempDir(), repo)
	if err != nil || exists {
		t.Fatalf("Load() = exists %v, error %v; want the defaults", exists, err)
	}
	if c != Default(repo) {
		t.Errorf("Load() = %+v, want the defaults", c)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("the defaults are invalid: %v", err)
	}
	// Today's behaviour: Codex is the Dev, Claude Code the PM and the
	// companion, each with the CLI's own model and effort.
	team, err := c.Team()
	if err != nil {
		t.Fatal(err)
	}
	for role, want := range map[session.Role]string{session.Dev: agents.CodexID, session.PM: agents.ClaudeID, session.Companion: agents.ClaudeID} {
		ch := team.For(role)
		if ch.CLI.ID != want || ch.Model != "" || ch.Effort != "" {
			t.Errorf("Team().For(%s) = %+v; want %s with defaults", role, ch, want)
		}
	}
	if c.Autonomy != 8 {
		t.Errorf("Autonomy = %d, want 8", c.Autonomy)
	}
}

func TestLoadFile(t *testing.T) {
	home := t.TempDir()
	write(t, home, `# Written by hand.
repo: sohaibseniorswe-sys/palantir
dir: /home/sweeng/projects/palantir
agents:
  dev:
    cli: codex          # codex | claude
    model: gpt-6.1-sol  # empty = the CLI's own default
    effort: medium
  pm:
    cli: claude
    model: opus
    effort: high
  companion:
    same_as_pm: false
    cli: claude
    model: sonnet
    effort: medium
autonomy: 6
`)
	c, exists, err := Load(home, repo)
	if err != nil || !exists {
		t.Fatalf("Load() = exists %v, error %v", exists, err)
	}
	want := Config{
		Repo: repo,
		Dir:  "/home/sweeng/projects/palantir",
		Agents: Agents{
			Dev:       Agent{CLI: "codex", Model: "gpt-6.1-sol", Effort: "medium"},
			PM:        Agent{CLI: "claude", Model: "opus", Effort: "high"},
			Companion: Companion{SameAsPM: false, Agent: Agent{CLI: "claude", Model: "sonnet", Effort: "medium"}},
		},
		Autonomy: 6,
	}
	if c != want {
		t.Errorf("Load() =\n%+v\nwant\n%+v", c, want)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
	team, err := c.Team()
	if err != nil {
		t.Fatal(err)
	}
	if ch := team.Companion; ch.CLI.ID != "claude" || ch.Model != "sonnet" || ch.Effort != "medium" {
		t.Errorf("Team().Companion = %+v", ch)
	}
	if ch := team.Dev; ch.CLI.ID != "codex" || ch.Model != "gpt-6.1-sol" || ch.Effort != "medium" {
		t.Errorf("Team().Dev = %+v", ch)
	}
}

func TestLoadKeepsDefaultsForMissingKeys(t *testing.T) {
	home := t.TempDir()
	write(t, home, "agents:\n  dev:\n    cli: claude\n")
	c, _, err := Load(home, repo)
	if err != nil {
		t.Fatal(err)
	}
	if c.Autonomy != 8 || c.Agents.PM.CLI != "claude" || c.Agents.Dev.CLI != "claude" || c.Repo != repo {
		t.Errorf("Load() = %+v, want the defaults with the Dev on Claude Code", c)
	}
}

func TestLoadRejectsUnknownKeysAndBadYAML(t *testing.T) {
	for name, text := range map[string]string{
		"typo":     "autonomyy: 3\n",
		"bad yaml": "agents: [\n",
		"bad type": "autonomy: high\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			write(t, home, text)
			if _, _, err := Load(home, repo); err == nil || !strings.Contains(err.Error(), "parse config") {
				t.Errorf("Load() error = %v, want a parse error", err)
			}
		})
	}
}

func TestEmptyFileMeansDefaults(t *testing.T) {
	home := t.TempDir()
	write(t, home, "# nothing yet\n")
	c, exists, err := Load(home, repo)
	if err != nil || !exists || c != Default(repo) {
		t.Errorf("Load(empty file) = %+v, %v, %v", c, exists, err)
	}
}

func TestSaveAndReload(t *testing.T) {
	home := t.TempDir()
	c := Default(repo)
	c.Dir = "/home/sweeng/projects/palantir"
	c.Agents.Dev = Agent{CLI: "claude", Model: "opus", Effort: "max"}
	c.Agents.PM = Agent{CLI: "codex", Model: "gpt-6.1-sol", Effort: "xhigh"}
	c.Agents.Companion = Companion{SameAsPM: false, Agent: Agent{CLI: "claude", Model: "haiku"}}
	c.Autonomy = 3
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	got, exists, err := Load(home, repo)
	if err != nil || !exists || got != c {
		t.Errorf("reload = %+v, %v, %v\nwant %+v", got, exists, err, c)
	}

	data, err := os.ReadFile(Path(home, repo))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, "# Huddle settings for one project.\n# Written by `huddle setup`. Safe to edit by hand.\n") {
		t.Errorf("the file does not start with the comment header:\n%s", text)
	}
	for _, want := range []string{"repo: sohaibseniorswe-sys/palantir\n", "    same_as_pm: false\n    cli: claude\n    model: haiku\n", "autonomy: 3\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("the file lacks %q:\n%s", want, text)
		}
	}
	// Saving again replaces the file; no temporary file is left behind.
	if err := Save(home, c); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(Path(home, repo)))
	if err != nil || len(entries) != 1 {
		t.Errorf("the folder holds %d files (%v), want only the config", len(entries), err)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		change  func(c *Config)
		wantErr string // "" means valid
	}{
		{"defaults", func(c *Config) {}, ""},
		{"swapped roles", func(c *Config) { c.Agents.Dev.CLI, c.Agents.PM.CLI = "claude", "codex" }, ""},
		{"invalid CLI", func(c *Config) { c.Agents.Dev.CLI = "gemini" }, `agents.dev.cli: unknown agent CLI "gemini"`},
		{"empty CLI", func(c *Config) { c.Agents.PM.CLI = "" }, `agents.pm.cli: unknown agent CLI ""`},
		{"codex effort on claude", func(c *Config) { c.Agents.PM.Effort = "none" }, `agents.pm.effort: "none" is not a Claude Code effort`},
		{"claude-only effort is fine on codex", func(c *Config) { c.Agents.Dev.Effort = "max" }, ""},
		{"unknown effort on codex", func(c *Config) { c.Agents.Dev.Effort = "extreme" }, `agents.dev.effort: "extreme" is not a Codex effort`},
		{"companion checked when its own", func(c *Config) {
			c.Agents.Companion = Companion{SameAsPM: false, Agent: Agent{CLI: "claude", Effort: "minimal"}}
		}, `agents.companion.effort: "minimal" is not a Claude Code effort`},
		{"companion ignored when same as PM", func(c *Config) {
			c.Agents.Companion = Companion{SameAsPM: true, Agent: Agent{CLI: "nonsense"}}
		}, ""},
		{"autonomy below range", func(c *Config) { c.Autonomy = -1 }, "autonomy: -1 is outside 0–10"},
		{"autonomy above range", func(c *Config) { c.Autonomy = 11 }, "autonomy: 11 is outside 0–10"},
		{"autonomy edges", func(c *Config) { c.Autonomy = 0 }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default(repo)
			tt.change(&c)
			err := c.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Validate() = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	c := Default(repo)
	c.Agents.Dev.CLI = "gemini"
	c.Autonomy = 12
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "agents.dev.cli") || !strings.Contains(err.Error(), "autonomy") {
		t.Errorf("Validate() = %v, want both problems", err)
	}
}

func TestAutonomyName(t *testing.T) {
	for level, want := range map[int]string{0: "Supervised", 2: "Supervised", 3: "Checkpoints", 5: "Checkpoints", 6: "Autonomous", 8: "Autonomous", 9: "Full autonomy", 10: "Full autonomy"} {
		if got := AutonomyName(level); got != want {
			t.Errorf("AutonomyName(%d) = %q, want %q", level, got, want)
		}
	}
}

func TestTeamCompanionFollowsThePM(t *testing.T) {
	c := Default(repo)
	c.Agents.PM = Agent{CLI: "codex", Model: "gpt-6.1-sol", Effort: "high"}
	c.Agents.Companion = Companion{SameAsPM: true, Agent: Agent{CLI: "claude", Model: "haiku"}}
	team, err := c.Team()
	if err != nil {
		t.Fatal(err)
	}
	if team.Companion.String() != team.PM.String() {
		t.Errorf("companion = %+v, want the PM's %+v", team.Companion, team.PM)
	}
	c.Agents.Dev.CLI = "gemini"
	if _, err := c.Team(); err == nil || !strings.Contains(err.Error(), "agents.dev.cli") {
		t.Errorf("Team() with an unknown CLI: error = %v", err)
	}
}
