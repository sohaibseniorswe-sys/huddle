// Package config is a project's settings file: which CLI, model and effort
// play each role (Dev, PM, companion), and the autonomy level. There is one
// YAML file per target repo, ~/.huddle/projects/<owner>_<repo>.yaml.
//
// `huddle setup` (the wizard, v3.4) writes the file; until then Huddle uses
// the defaults, which are what v1 and v2 always ran: Codex as the Dev,
// Claude Code as the PM, the companion on the PM's CLI. The file is plain
// YAML with comments and safe to edit by hand.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
)

// DefaultAutonomy is the autonomy level when the file does not set one.
const DefaultAutonomy = 8

// The autonomy dial's range.
const (
	MinAutonomy = 0
	MaxAutonomy = 10
)

// header opens every file Save writes. yaml.v3 does not keep the comments
// of a file it reads, so Save writes this one each time.
const header = `# Huddle settings for one project.
# Written by ` + "`huddle setup`" + `. Safe to edit by hand.
#
# agents.<role>.cli: codex | claude
# agents.<role>.model: a model name or alias; empty = the CLI's own default
# agents.<role>.effort: codex: none | minimal | low | medium | high | xhigh | max
#                       claude: low | medium | high | xhigh | max
#                       empty = the CLI's own default
# agents.companion.same_as_pm: true = the companion uses the PM's cli, model and effort
# autonomy: 0 (most careful) to 10 (most independent)
`

// Config is one project's settings. The yaml tags are the keys in the file.
type Config struct {
	Repo     string `yaml:"repo"` // "owner/name"
	Dir      string `yaml:"dir"`  // the local clone; may be empty
	Agents   Agents `yaml:"agents"`
	Autonomy int    `yaml:"autonomy"` // 0–10; used from v3.5
}

// Agents says what plays each role.
type Agents struct {
	Dev       Agent     `yaml:"dev"`
	PM        Agent     `yaml:"pm"`
	Companion Companion `yaml:"companion"`
}

// Agent is one role's CLI, model and effort, as stored. An empty Model or
// Effort means the CLI's own default.
type Agent struct {
	CLI    string `yaml:"cli"` // agents.CodexID or agents.ClaudeID
	Model  string `yaml:"model"`
	Effort string `yaml:"effort"`
}

// Companion is the companion's settings: either the PM's (SameAsPM), or
// its own. yaml's "inline" puts Agent's keys next to same_as_pm in the
// file instead of under a key of their own; it is not inheritance.
type Companion struct {
	SameAsPM bool  `yaml:"same_as_pm"`
	Agent    Agent `yaml:",inline"`
}

// Default returns the settings Huddle uses for repo when it has no file.
func Default(repo string) Config {
	return Config{
		Repo: repo,
		Agents: Agents{
			Dev: Agent{CLI: agents.CodexID},
			PM:  Agent{CLI: agents.ClaudeID},
			// The companion's own fields are filled too, so a hand edit
			// that turns same_as_pm off starts from something valid.
			Companion: Companion{SameAsPM: true, Agent: Agent{CLI: agents.ClaudeID}},
		},
		Autonomy: DefaultAutonomy,
	}
}

// Path returns the settings file of repo ("owner/name"):
// <home>/.huddle/projects/<owner>_<repo>.yaml.
func Path(home, repo string) string {
	return filepath.Join(home, ".huddle", "projects", strings.ReplaceAll(repo, "/", "_")+".yaml")
}

// Load reads the settings file of repo. If there is none, it returns
// Default(repo) and exists = false. A file that cannot be read or parsed,
// or that names a key Huddle does not know (a typo, say), is an error. Load
// does not validate the values; call Validate.
func Load(home, repo string) (c Config, exists bool, err error) {
	path := Path(home, repo)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(repo), false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	// Start from the defaults, so a key the file leaves out keeps its
	// default (autonomy 8, say) instead of becoming zero.
	c = Default(repo)
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, true, nil
}

// Save writes c to the settings file of c.Repo, with the comment header.
// It writes a temporary file and renames it over the old one, so a crash
// never leaves half a file.
func Save(home string, c Config) error {
	path := Path(home, c.Repo)
	// Two spaces per level, as people indent YAML by hand (yaml.v3's
	// own default is four).
	var body bytes.Buffer
	enc := yaml.NewEncoder(&body)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config folder: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte(header+"\n"), body.Bytes()...), 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// Validate checks every value: each CLI is known, each effort is one of
// its CLI's, and the autonomy level is 0–10. It returns all problems at
// once, joined, each naming its key.
func (c Config) Validate() error {
	var errs []error
	check := func(key string, a Agent) {
		cli, err := agents.ByID(a.CLI)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s.cli: %w", key, err))
			return
		}
		if !cli.ValidEffort(a.Effort) {
			errs = append(errs, fmt.Errorf("%s.effort: %q is not a %s effort (want one of %s, or empty for the CLI's default)",
				key, a.Effort, cli.Name, strings.Join(cli.Efforts, ", ")))
		}
	}
	check("agents.dev", c.Agents.Dev)
	check("agents.pm", c.Agents.PM)
	if !c.Agents.Companion.SameAsPM {
		check("agents.companion", c.Agents.Companion.Agent)
	}
	if c.Autonomy < MinAutonomy || c.Autonomy > MaxAutonomy {
		errs = append(errs, fmt.Errorf("autonomy: %d is outside %d–%d", c.Autonomy, MinAutonomy, MaxAutonomy))
	}
	return errors.Join(errs...)
}

// Team returns what plays each role: the CLI with its model and effort.
// The companion gets the PM's when SameAsPM is set. Call Validate first: an
// unknown CLI is an error here too.
func (c Config) Team() (agents.Team, error) {
	companion := c.Agents.Companion.Agent
	if c.Agents.Companion.SameAsPM {
		companion = c.Agents.PM
	}
	var t agents.Team
	var err error
	if t.Dev, err = choice("agents.dev", c.Agents.Dev); err != nil {
		return agents.Team{}, err
	}
	if t.PM, err = choice("agents.pm", c.Agents.PM); err != nil {
		return agents.Team{}, err
	}
	if t.Companion, err = choice("agents.companion", companion); err != nil {
		return agents.Team{}, err
	}
	return t, nil
}

// choice turns one stored role into an agents.Choice. key names the role
// in the error.
func choice(key string, a Agent) (agents.Choice, error) {
	cli, err := agents.ByID(a.CLI)
	if err != nil {
		return agents.Choice{}, fmt.Errorf("%s.cli: %w", key, err)
	}
	return agents.Choice{CLI: cli, Model: a.Model, Effort: a.Effort}, nil
}

// AutonomyName is the name of level's band on the autonomy dial
// (docs/v3/issues/05-autonomy-level.md): "Supervised", "Checkpoints",
// "Autonomous" or "Full autonomy".
func AutonomyName(level int) string {
	switch {
	case level <= 2:
		return "Supervised"
	case level <= 5:
		return "Checkpoints"
	case level <= 8:
		return "Autonomous"
	default:
		return "Full autonomy"
	}
}
