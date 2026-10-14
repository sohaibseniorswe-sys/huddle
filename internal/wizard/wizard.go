// Package wizard is Huddle's setup wizard (docs/v3/issues/04-setup-wizard.md):
// nine steps that find the project folder and its GitHub repo, choose the
// CLI, model and effort of each role, check the logins with a short test
// run of every choice, set the autonomy level, prepare the repo (labels,
// AGENTS.md, CLAUDE.md) and finally write the project's settings file
// (package config).
//
// It runs the first time Huddle is used in a project, and again with
// `huddle setup`. Like the run screen (package app) it is a Bubble Tea
// model: Update receives every key, mouse event and finished background
// job as a message and returns the changed model; View draws it. Anything
// slow (git, gh, agent CLIs) runs in a tea.Cmd, a function Bubble Tea runs
// on another goroutine, whose result comes back to Update as a message.
//
// Nothing is written before the last step: Ctrl+C at any step leaves the
// project as it was, apart from labels step 8 creates on GitHub.
package wizard

import (
	"context"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
)

// Deps is everything the wizard touches outside the process. main passes
// the real ones; tests pass fakes.
type Deps struct {
	Home   string // the user's home folder: the settings file lives below it
	Target string // the repo being set up, "owner/name"

	Getwd     func() (string, error)
	LookupEnv func(string) (string, bool) // for the API key check
	// TopLevel returns the root of the git work tree containing dir.
	TopLevel func(ctx context.Context, dir string) (string, error)
	// RemoteURL returns the URL of a remote of the repo in dir.
	RemoteURL func(ctx context.Context, dir, remote string) (string, error)
	// Commit stages and commits files in dir.
	Commit func(ctx context.Context, dir, message string, files ...string) error
	GH     gh.Client
	// CheckCLI checks that binary ("codex" or "claude") is installed and
	// signed in with a subscription, and returns its version.
	CheckCLI func(ctx context.Context, binary string) (version string, err error)
	// Validate runs a short test of cli with model and effort
	// (agents.Validate).
	Validate func(ctx context.Context, cli agents.CLI, model, effort string) error
}

// The steps, numbered as on screen.
const (
	stepFolder   = 1
	stepRepo     = 2
	stepDev      = 3
	stepPM       = 4
	stepComp     = 5
	stepChecks   = 6
	stepAutonomy = 7
	stepPrepare  = 8
	stepSave     = 9
	stepCount    = 9
)

// stepNames are the names in the step list, indexed by step - 1.
var stepNames = [stepCount]string{
	"Project folder", "GitHub repo", "Dev agent", "PM agent", "Companion",
	"Logins", "Autonomy level", "Repo preparation", "Save",
}

// The three roles, as indexes into Model.roles.
const (
	roleDev = iota
	rolePM
	roleComp
	roleCount
)

// roleNames are the roles as the wizard names them.
var roleNames = [roleCount]string{"Dev", "PM", "Companion"}

// roleStep is the step that chooses role's agent.
func roleStep(role int) int { return stepDev + role }

// cliOrder is the order of the CLI chips, as on screen 11.
var cliOrder = []agents.CLI{agents.Claude(), agents.Codex()}

// agentChoice is what the human has chosen for one role: indexes into the
// chips, plus the text of the Custom… model name.
type agentChoice struct {
	cli    int // index in cliOrder
	model  int // index in the CLI's KnownModels; len(KnownModels) is Custom…
	effort int // index in the CLI's Efforts
	custom textinput.Model
}

// Model is the wizard's Bubble Tea model. Update returns a changed copy;
// Bubble Tea keeps the copy and passes it to the next Update and to View.
type Model struct {
	deps     Deps
	original config.Config // the settings the wizard started from
	width    int
	height   int
	step     int
	focus    int // index of the focused field in m.fields()
	spinner  spinner.Model

	// Step 1: the project folder.
	folder     textinput.Model
	folderErr  string
	dir        string // the git repo's root once step 1 is confirmed
	checking   bool   // a background check of this step is running
	folderSeen bool   // the detection at startup has finished

	// Step 2: the GitHub repo.
	repo       repoResult
	repoDone   bool
	repoErr    string
	repoForDir string // the folder repo was checked for

	// Steps 3–5.
	roles         [roleCount]agentChoice
	companionSame bool

	// Step 6: the checks, run one at a time.
	checks    []checkRow
	checkRun  int             // counts the times step 6 started; old results are dropped
	validated map[string]bool // choices that passed a test run, by choiceKey
	logins    []string        // "Codex: ChatGPT", for the step list

	// Step 7.
	autonomy int

	// Step 8.
	prep     prepResult
	prepDone bool
	prepErr  string
	create   map[string]bool // starter file name → create it

	// Step 9.
	commit  bool // commit the new agent files
	saveErr string
	saved   bool
	result  config.Config
}

// New returns the wizard with the values of c pre-selected. c is the
// project's current settings, or config.Default when it has none.
func New(deps Deps, c config.Config) Model {
	m := Model{
		deps:          deps,
		original:      c,
		step:          stepFolder,
		autonomy:      c.Autonomy,
		companionSame: c.Agents.Companion.SameAsPM,
		validated:     map[string]bool{},
		create:        map[string]bool{},
		commit:        true,
		width:         100,
		height:        34,
	}
	m.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	m.folder = textinput.New()
	m.folder.Prompt = ""
	// A cursor that does not blink: blinking needs a timer message
	// several times a second, for nothing a setup screen needs.
	m.folder.Cursor.SetMode(cursor.CursorStatic)
	m.folder.SetValue(c.Dir)
	m.folder.Focus()
	m.roles[roleDev] = fromConfig(c.Agents.Dev)
	m.roles[rolePM] = fromConfig(c.Agents.PM)
	m.roles[roleComp] = fromConfig(c.Agents.Companion.Agent)
	return m
}

// Init starts the detection of the git repo in the current folder, and the
// spinner. Bubble Tea calls it once, before the first Update.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.detectFolder(), m.spinner.Tick)
}

// Saved reports whether the wizard wrote the settings, and returns them.
// It is false when the human quit with Ctrl+C.
func (m Model) Saved() (config.Config, bool) { return m.result, m.saved }

// fromConfig turns one stored role into chip indexes. An unknown CLI falls
// back to the first; an empty model selects the CLI's first known model;
// a model the table does not know becomes Custom… with that name; an
// empty effort selects the CLI's suggested effort.
func fromConfig(a config.Agent) agentChoice {
	c := agentChoice{custom: newCustomInput()}
	c.cli = max(slices.IndexFunc(cliOrder, func(cli agents.CLI) bool { return cli.ID == a.CLI }), 0)
	cli := cliOrder[c.cli]
	if a.Model != "" {
		c.model = slices.IndexFunc(cli.KnownModels, func(k agents.Model) bool { return k.Value == a.Model })
		if c.model < 0 {
			c.model = len(cli.KnownModels)
			c.custom.SetValue(a.Model)
		}
	}
	c.effort = effortIndex(cli, a.Effort)
	return c
}

// newCustomInput returns the text input for a Custom… model name.
func newCustomInput() textinput.Model {
	t := textinput.New()
	t.Prompt = ""
	t.Placeholder = "model name"
	t.CharLimit = 100
	t.Cursor.SetMode(cursor.CursorStatic)
	return t
}

// effortIndex is the index of effort in cli's efforts, or of the CLI's
// suggested effort when effort is empty or not one of them.
func effortIndex(cli agents.CLI, effort string) int {
	if i := slices.Index(cli.Efforts, effort); i >= 0 {
		return i
	}
	return max(slices.Index(cli.Efforts, cli.DefaultEffort), 0)
}

func (c agentChoice) agentCLI() agents.CLI { return cliOrder[c.cli] }

// isCustom reports whether Custom… is the selected model chip.
func (c agentChoice) isCustom() bool { return c.model >= len(c.agentCLI().KnownModels) }

// modelValue is the model name passed to the CLI.
func (c agentChoice) modelValue() string {
	if c.isCustom() {
		return strings.TrimSpace(c.custom.Value())
	}
	return c.agentCLI().KnownModels[c.model].Value
}

// modelLabel is the model as shown: the known model's label, or the
// custom name.
func (c agentChoice) modelLabel() string {
	if c.isCustom() {
		return strings.TrimSpace(c.custom.Value())
	}
	return c.agentCLI().KnownModels[c.model].Label
}

func (c agentChoice) effortValue() string { return c.agentCLI().Efforts[c.effort] }

// summary is the choice in one line: "Codex · GPT-6.1 Sol · effort medium".
func (c agentChoice) summary() string {
	return c.agentCLI().Name + " · " + c.modelLabel() + " · effort " + c.effortValue()
}

// stored is the choice as the settings file keeps it.
func (c agentChoice) stored() config.Agent {
	return config.Agent{CLI: c.agentCLI().ID, Model: c.modelValue(), Effort: c.effortValue()}
}

// choiceKey identifies a CLI, model and effort, for the cache of passed
// test runs.
func (c agentChoice) choiceKey() string {
	return c.agentCLI().ID + "|" + c.modelValue() + "|" + c.effortValue()
}

// setCLI selects CLI chip i and resets the model and effort to that CLI's
// first known model and suggested effort: the old ones belong to the old
// CLI.
func (c *agentChoice) setCLI(i int) {
	if i == c.cli {
		return
	}
	c.cli = i
	c.model = 0
	c.effort = effortIndex(c.agentCLI(), "")
}

// config returns the settings as chosen so far.
func (m Model) config() config.Config {
	c := m.original
	c.Repo = m.deps.Target
	c.Dir = m.dir
	c.Agents.Dev = m.roles[roleDev].stored()
	c.Agents.PM = m.roles[rolePM].stored()
	c.Agents.Companion.SameAsPM = m.companionSame
	if m.companionSame {
		// Keep the PM's values in the companion's own fields, so a hand
		// edit that turns same_as_pm off starts from something valid.
		c.Agents.Companion.Agent = c.Agents.PM
	} else {
		c.Agents.Companion.Agent = m.roles[roleComp].stored()
	}
	c.Autonomy = m.autonomy
	return c
}
