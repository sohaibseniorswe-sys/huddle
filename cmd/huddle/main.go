// Command huddle runs a Dev agent and a PM agent in turns through a GitHub
// milestone, with a companion agent the human talks to on the run screen.
// Each role is played by Codex or Claude Code, as the project's settings
// file says (package config); by default Codex is the Dev and Claude Code
// the PM. See MVP_PLAN.md and docs/v3.
//
// The first time it runs for a repo (no settings file yet), and with
// `huddle setup`, it shows the setup wizard (package wizard). After the
// startup checks it shows the start menu (package start): plan new
// milestones, run an existing one or resume an interrupted one. Then the
// run screen (package app) runs the milestone, and every queued milestone
// after it. --resume, --run and --plan skip the menu. With --plain it
// prints the loop's lines to stdout as Huddle v1 did.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/agents"
	"github.com/sohaibseniorswe-sys/huddle/internal/app"
	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/companion"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/gitx"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
	"github.com/sohaibseniorswe-sys/huddle/internal/start"
	"github.com/sohaibseniorswe-sys/huddle/internal/state"
	"github.com/sohaibseniorswe-sys/huddle/internal/store"
	"github.com/sohaibseniorswe-sys/huddle/internal/wizard"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.1.0-dev"

// deps holds everything run touches outside the process, so tests can fake it.
type deps struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	getwd     func() (string, error)
	lookupEnv func(string) (string, bool)
	gh        gh.Client
	agents    checks.AgentCLIs
	// runAgents returns the agents for the watch loop, run in repo root
	// dir and played as team says. stream receives their raw output (nil
	// for the run screen).
	runAgents func(dir, home string, team agents.Team, stream io.Writer) loop.Agents
	clock     loop.Clock
	// runUI shows the run screen and returns the last loop's exit code,
	// and whether to show the start menu again (app.Run).
	runUI func(ctx context.Context, cfg app.Config, stderr io.Writer) (code int, startMenu bool)
	// runStart shows the start menu and returns the human's choice
	// (start.Run).
	runStart func(ctx context.Context, facts start.Facts, settings string) (start.Choice, error)
	// runWizard shows the setup wizard with c pre-selected and returns
	// the settings it saved (wizard.Run).
	runWizard func(ctx context.Context, wd wizard.Deps, c config.Config) (config.Config, error)
	// validate is the wizard's test run of a CLI, model and effort.
	validate func(ctx context.Context, cli agents.CLI, model, effort string) error
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], deps{
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		getwd:     os.Getwd,
		lookupEnv: os.LookupEnv,
		gh:        gh.CLI{},
		agents:    checks.AgentCLIs{LookPath: exec.LookPath, Run: checks.ExecRun},
		runAgents: func(dir, home string, team agents.Team, stream io.Writer) loop.Agents {
			return loop.CLIAgents{Dir: dir, Home: home, Team: team, Stream: stream}
		},
		clock:     loop.RealClock{},
		runUI:     app.Run,
		runStart:  start.Run,
		runWizard: wizard.Run,
		validate:  agents.Validate,
	}))
}

type options struct {
	home       string
	dataRemote string
	setup      bool // `huddle setup`: run the setup wizard
	version    bool
	yes        bool
	once       bool
	plain      bool
	repo       string
	// autonomy is --autonomy, the level for this run only; autonomySet
	// says whether the flag was given at all, because 0 is a valid level.
	autonomy    int
	autonomySet bool
	// The start modes that skip the start menu; at most one is set.
	resume   bool   // --resume: resume the interrupted milestone
	runTitle string // --run "<milestone>": run that open milestone
	plan     bool   // --plan: plan new milestones with the companion
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("huddle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.home, "home", "", "state root (overrides HUDDLE_HOME)")
	fs.StringVar(&o.dataRemote, "data-remote", "", "huddle-data git remote to clone when absent")
	fs.BoolVar(&o.version, "version", false, "print the version and exit")
	fs.BoolVar(&o.yes, "yes", false, "skip the confirmation prompt")
	fs.BoolVar(&o.once, "once", false, "run a single turn, then stop (with --plain: exit)")
	fs.BoolVar(&o.plain, "plain", false, "print plain lines to stdout instead of the full-screen UI (as v1 did)")
	fs.StringVar(&o.repo, "repo", checks.DefaultRepo, "target repo as owner/name (for testing against the sandbox)")
	// The default 0 is never used (autonomySet says whether the flag was
	// given); a zero default keeps "(default 0)" out of the help text.
	fs.IntVar(&o.autonomy, "autonomy", 0, "autonomy level for this run only, 0 (most careful) to 10 (most independent); overrides the settings file")
	fs.BoolVar(&o.resume, "resume", false, "resume the interrupted milestone without showing the start menu")
	fs.StringVar(&o.runTitle, "run", "", "run the open milestone with this title without showing the start menu")
	fs.BoolVar(&o.plan, "plan", false, "plan new milestones with the companion without showing the start menu")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: huddle [flags]\n       huddle [flags] setup\n\nRun from inside a local clone of the target repo.\nWithout --resume, --run or --plan, Huddle opens on the start menu.\n`huddle setup` runs the setup wizard again.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	// The flag package stops at the first argument that is not a flag, so
	// `huddle setup --repo x` keeps its flags after "setup": parse them too.
	args = fs.Args()
	if len(args) > 0 && args[0] == "setup" {
		o.setup = true
		if err := fs.Parse(args[1:]); err != nil {
			return o, err
		}
		args = fs.Args()
	}
	if len(args) > 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
	}
	if err := checks.ValidateRepo(o.repo); err != nil {
		return o, fmt.Errorf("--repo: %w", err)
	}
	// fs.Visit calls its function only for the flags given on the command
	// line, so it tells "--autonomy 8" apart from the default.
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "autonomy" {
			o.autonomySet = true
		}
	})
	if o.autonomySet && (o.autonomy < config.MinAutonomy || o.autonomy > config.MaxAutonomy) {
		return o, fmt.Errorf("--autonomy: %d is outside %d–%d (0 is the most careful, 10 the most independent)",
			o.autonomy, config.MinAutonomy, config.MaxAutonomy)
	}
	modes := 0
	for _, set := range []bool{o.resume, o.runTitle != "", o.plan} {
		if set {
			modes++
		}
	}
	if modes > 1 {
		return o, errors.New("use only one of --resume, --run and --plan")
	}
	if o.plain && (o.plan || o.runTitle != "") {
		return o, errors.New("--plan and --run need the run screen; leave out --plain")
	}
	return o, nil
}

func run(ctx context.Context, args []string, d deps) int {
	opts, err := parseFlags(args, d.stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(d.stderr, "huddle: %v\n", err)
		return 2
	}
	if opts.version {
		fmt.Fprintf(d.stdout, "huddle %s\n", version)
		return 0
	}

	ok := func(format string, a ...any) { fmt.Fprintf(d.stdout, "✔ "+format+"\n", a...) }
	fail := func(err error) int {
		fmt.Fprintf(d.stderr, "✘ %v\n", err)
		return 1
	}

	// Check 1, the billing guard: nothing else runs while an API key is set.
	if err := checks.CheckAPIKeys(d.lookupEnv); err != nil {
		return fail(err)
	}

	target := opts.repo
	if target != checks.DefaultRepo {
		fmt.Fprintf(d.stdout, "target: %s (override)\n", target)
	}

	// The project's settings: which CLI plays each role. Without a
	// settings file, or with `huddle setup`, the wizard writes one first.
	home, err := state.Resolve(opts.home, d.lookupEnv)
	if err != nil {
		return fail(err)
	}
	if _, err := store.Open(ctx, home, opts.dataRemote); err != nil {
		return fail(err)
	}
	settingsPath := tildePath(config.Path(home, target), d.lookupEnv)
	cfg, exists, err := config.Load(home, target)
	if err != nil {
		return fail(err)
	}
	if opts.setup || !exists {
		if opts.plain {
			// The wizard needs a terminal.
			fmt.Fprintln(d.stderr, "Run huddle setup first")
			return 2
		}
		cfg, err = d.runWizard(ctx, d.wizardDeps(home, target), cfg)
		if errors.Is(err, wizard.ErrCanceled) {
			fmt.Fprintln(d.stdout, err)
			return 1
		}
		if err != nil {
			return fail(err)
		}
		exists = true
		ok("settings saved to %s", settingsPath)
		if opts.setup {
			return 0
		}
	}

	// --autonomy replaces the settings' level for this run only; the file
	// is not changed. It is applied after the wizard, so `huddle setup`
	// still shows and saves the file's own level.
	if opts.autonomySet {
		fmt.Fprintf(d.stdout, "autonomy: %d (--autonomy; the settings say %d)\n", opts.autonomy, cfg.Autonomy)
		cfg.Autonomy = opts.autonomy
	}

	// Check 2: inside a git repo whose origin is the target. Outside one,
	// the folder in the settings is used.
	wd, err := d.getwd()
	if err != nil {
		return fail(err)
	}
	root, err := gitx.TopLevel(ctx, wd)
	if err != nil && cfg.Dir != "" {
		root, err = gitx.TopLevel(ctx, cfg.Dir)
	}
	if err != nil {
		return fail(fmt.Errorf("not inside a git repo: %w", err))
	}
	remote, err := gitx.RemoteURL(ctx, root, "origin")
	if err != nil {
		return fail(fmt.Errorf("no origin remote: %w", err))
	}
	if err := checks.MatchRemote(remote, target); err != nil {
		return fail(err)
	}
	ok("git repo %s, remote %s", tildePath(root, d.lookupEnv), target)

	// Check 3: agent files. There is no clean-tree check: after an
	// interrupted run the tree is normally dirty, and the agent resumes.
	created, err := checks.EnsureAgentFiles(root)
	if err != nil {
		return fail(err)
	}
	if created {
		ok("AGENTS.md and CLAUDE.md present, created empty %s", checks.SessionFile)
	} else {
		ok("AGENTS.md, CLAUDE.md and %s present", checks.SessionFile)
	}

	// Checks 4 and 5: gh login, repo access, labels.
	if err := checks.CheckGitHub(ctx, d.gh, target); err != nil {
		return fail(err)
	}
	newLabels, err := checks.EnsureLabels(ctx, d.gh, target)
	if err != nil {
		return fail(err)
	}
	if len(newLabels) > 0 {
		ok("gh logged in, repo access ok, labels ready (created %s)", strings.Join(newLabels, ", "))
	} else {
		ok("gh logged in, repo access ok, labels ready")
	}

	// Check 6: the settings are valid.
	if err := cfg.Validate(); err != nil {
		return fail(fmt.Errorf("settings %s: %w", settingsPath, err))
	}
	team, err := cfg.Team()
	if err != nil {
		return fail(err) // Validate has checked the CLIs, so this does not happen
	}
	ok("settings %s", settingsPath)
	ok("Dev: %s; PM: %s; companion: %s", team.Dev, team.PM, team.Companion)
	ok("autonomy %d · %s", cfg.Autonomy, config.AutonomyName(cfg.Autonomy))

	// Check 7: the agent CLIs in use, with subscription logins.
	bins := usedBinaries(team)
	versions, err := d.agents.Check(ctx, bins)
	if err != nil {
		return fail(err)
	}
	ok("%s installed with subscription logins, no API keys in env", strings.Join(bins, " and "))

	// Both questions below read the same input. bufio.NewReader hands
	// back a *bufio.Reader unchanged, so neither loses what the other
	// buffered.
	in := bufio.NewReader(d.stdin)

	// Check 8: a control issue closed while its milestone still has open
	// issues is offered for reopening (with --yes it is reopened). This is
	// asked only when no control issue is open at all.
	reopened, stop, err := reopenUnfinished(ctx, d, target, in, opts.yes, ok)
	if err != nil {
		return fail(err)
	}
	if stop {
		return 0
	}

	s := starter{d: d, opts: opts, cfg: cfg, team: team, versions: versions, home: home, target: target, root: root, exists: exists}
	if opts.plain {
		return s.runPlain(ctx, in, reopened)
	}
	return s.runScreens(ctx)
}

// reopenUnfinished is startup check 8 for a repo with no open control
// issue: if a control issue was closed while its milestone still has open
// issues, it asks whether to reopen it (yes reopens without asking).
// stop is true when the human chose to leave it closed and exit.
func reopenUnfinished(ctx context.Context, d deps, target string, in *bufio.Reader, yes bool, ok func(string, ...any)) (reopened *gh.Issue, stop bool, err error) {
	_, err = checks.FindControlIssue(ctx, d.gh, target)
	if !errors.Is(err, checks.ErrNoControlIssue) {
		return nil, false, nil // one or more open control issues: nothing to reopen
	}
	closed, open, found, err := checks.FindUnfinishedControl(ctx, d.gh, target)
	if err != nil || !found {
		return nil, false, err
	}
	if !yes && !checks.ChooseReopen(in, d.stdout, closed.Number, open) {
		fmt.Fprintf(d.stdout, "left control issue #%d closed\n", closed.Number)
		return nil, true, nil
	}
	if err := d.gh.ReopenIssue(ctx, target, closed.Number); err != nil {
		return nil, false, fmt.Errorf("reopen control issue #%d: %w", closed.Number, err)
	}
	ok("reopened control issue #%d", closed.Number)
	closed.Closed = false
	return &closed, false, nil
}

// starter starts milestones once the startup checks have passed: on the
// run screen (with the start menu) or, with --plain, as Huddle v1 did.
type starter struct {
	d        deps
	opts     options
	cfg      config.Config
	team     agents.Team
	versions map[string]string
	home     string
	target   string
	root     string // the repo's local clone
	exists   bool   // the settings file exists
}

// runPlain is v1's start: the single open control issue, a confirmation,
// then the loop printing to stdout. reopened is the control issue check 8
// reopened, if any.
func (s starter) runPlain(ctx context.Context, in *bufio.Reader, reopened *gh.Issue) int {
	d := s.d
	var control gh.Issue
	var err error
	if reopened != nil {
		control = *reopened
	} else if control, err = checks.FindControlIssue(ctx, d.gh, s.target); err != nil {
		fmt.Fprintf(d.stderr, "✘ %v\n", err)
		return 1
	}
	state := "none"
	if st := checks.States(control); len(st) > 0 {
		state = strings.Join(st, ", ")
	}
	fmt.Fprintf(d.stdout, "✔ control issue #%d %q, milestone %q, state: %s\n", control.Number, control.Title, control.Milestone, state)

	// Check 9: confirmation.
	if !s.opts.yes && !checks.Confirm(in, d.stdout) {
		fmt.Fprintln(d.stdout, "aborted")
		return 1
	}
	l := s.newLoop(control, d.stdout)
	l.Report = loop.PlainReporter{Out: d.stdout}
	l.Controls = &loop.Controls{}
	return l.Run(ctx)
}

// newLoop returns the watch loop for control. stream receives the agents'
// raw output (nil on the run screen, where nothing may write to the
// terminal but the screen). The caller sets Report and Controls.
func (s starter) newLoop(control gh.Issue, stream io.Writer) *loop.Loop {
	return &loop.Loop{
		Control:   gh.Control{Client: s.d.gh, Repo: s.target, Number: control.Number},
		Milestone: control.Milestone,
		Agents:    s.d.runAgents(s.root, s.home, s.team, stream),
		Team:      s.team,
		Autonomy:  s.cfg.Autonomy,
		Sessions:  session.New(s.home, s.target, control.Number),
		Clock:     s.d.clock,
		Once:      s.opts.once,
	}
}

// runScreens shows the start menu (unless a start flag chose already) and
// the run screen, again and again: when every milestone is done and the
// queue is empty, a key press on the run screen brings the menu back.
func (s starter) runScreens(ctx context.Context) int {
	queue := start.NewQueue(s.home, s.target)
	for {
		ui, ok, code := s.choose(ctx, queue)
		if !ok {
			return code
		}
		code, again := s.d.runUI(ctx, ui, s.d.stderr)
		if !again {
			return code
		}
		// Back to the start menu: the flags chose only the first time.
		s.opts.resume, s.opts.runTitle, s.opts.plan = false, "", false
	}
}

// choose finds out what to run, from a start flag or the start menu, and
// returns the run screen's config. ok is false when Huddle should exit
// with code instead.
func (s starter) choose(ctx context.Context, queue start.Queue) (cfg app.Config, ok bool, code int) {
	d := s.d
	queued, err := queue.Load()
	if err != nil {
		fmt.Fprintf(d.stderr, "✘ %v\n", err)
		return app.Config{}, false, 1
	}
	facts, err := start.Load(ctx, d.gh, s.target, tildePath(s.root, d.lookupEnv), queued)
	if err != nil {
		fmt.Fprintf(d.stderr, "✘ %v\n", err)
		return app.Config{}, false, 1
	}

	var choice start.Choice
	switch {
	case s.opts.resume:
		if facts.Resume == nil {
			fmt.Fprintf(d.stderr, "✘ --resume: no open control issue on %s has a state:*-working, -ready or human-required label\n", s.target)
			return app.Config{}, false, 1
		}
		choice = start.Choice{Kind: start.KindResume, Resume: *facts.Resume}
	case s.opts.runTitle != "":
		m, found := facts.FindMilestone(s.opts.runTitle)
		if !found {
			fmt.Fprintf(d.stderr, "✘ --run: %s has no open milestone titled %q\n", s.target, s.opts.runTitle)
			return app.Config{}, false, 1
		}
		choice = start.Choice{Kind: start.KindRun, Milestone: m}
	case s.opts.plan:
		choice = start.Choice{Kind: start.KindPlan}
	default:
		choice, err = d.runStart(ctx, facts, app.SettingsText(s.screenConfig(gh.Issue{})))
		if err != nil {
			fmt.Fprintf(d.stderr, "huddle: %v\n", err)
			return app.Config{}, false, 1
		}
	}

	switch choice.Kind {
	case start.KindResume:
		return s.screenConfig(choice.Resume), true, 0
	case start.KindRun:
		m := choice.Milestone
		number, err := start.Prepare(ctx, d.gh, s.target, m.Title, m.Control, choice.Message)
		if err != nil {
			fmt.Fprintf(d.stderr, "✘ %v\n", err)
			return app.Config{}, false, 1
		}
		return s.screenConfig(gh.Issue{Number: number, Milestone: m.Title}), true, 0
	case start.KindPlan:
		cfg := s.screenConfig(gh.Issue{})
		cfg.Planning = true
		return cfg, true, 0
	}
	return app.Config{}, false, 0 // Quit
}

// screenConfig returns the run screen's config with control as the first
// milestone to run. A zero control (plan mode, or the settings page of
// the start menu) keeps the companion's session and conversation in files
// of their own, numbered 0.
func (s starter) screenConfig(control gh.Issue) app.Config {
	d := s.d
	userHome, _ := d.lookupEnv("HOME")
	return app.Config{
		Repo:      s.target,
		Milestone: control.Milestone,
		Control:   gh.Control{Client: d.gh, Repo: s.target, Number: control.Number},
		Team:      s.team,
		Settings: app.Settings{
			Home:               s.home,
			UserHome:           userHome,
			Path:               config.Path(s.home, s.target),
			Exists:             s.exists,
			Autonomy:           s.cfg.Autonomy,
			CompanionFollowsPM: s.cfg.Agents.Companion.SameAsPM,
			Versions:           s.versions,
			Sessions:           session.New(s.home, s.target, control.Number),
			CompanionSessions:  session.NewCompanion(s.home, s.target, control.Number),
		},
		LogDir:  runner.LogDir(s.home),
		RepoDir: s.root,
		Now:     time.Now,
		// The companion runs in the same clone, read-only.
		Companion: companion.Agent{
			Choice:    s.team.Companion,
			Dir:       s.root,
			Home:      s.home,
			Milestone: control.Milestone,
			Sessions:  session.NewCompanion(s.home, s.target, control.Number),
			Run:       runner.Run,
			Now:       time.Now,
		},
		History: companion.NewHistory(s.home, s.target, control.Number),
		First:   control,
		NewLoop: func(c gh.Issue) *loop.Loop { return s.newLoop(c, nil) },
		Queue:   start.NewQueue(s.home, s.target),
	}
}

// wizardDeps returns what the setup wizard needs, from d.
func (d deps) wizardDeps(home, target string) wizard.Deps {
	return wizard.Deps{
		Home:      home,
		Target:    target,
		Getwd:     d.getwd,
		LookupEnv: d.lookupEnv,
		TopLevel:  gitx.TopLevel,
		RemoteURL: gitx.RemoteURL,
		Commit:    gitx.Commit,
		GH:        d.gh,
		CheckCLI: func(ctx context.Context, binary string) (string, error) {
			versions, err := d.agents.Check(ctx, []string{binary})
			return versions[binary], err
		},
		Validate: d.validate,
	}
}

// usedBinaries lists the CLIs that play a role in team, each once, in
// role order: the ones the startup check must find.
func usedBinaries(team agents.Team) []string {
	var bins []string
	for _, c := range []agents.Choice{team.Dev, team.PM, team.Companion} {
		if !slices.Contains(bins, c.CLI.Binary) {
			bins = append(bins, c.CLI.Binary)
		}
	}
	return bins
}

// tildePath shortens a path under $HOME to start with "~".
func tildePath(p string, lookupEnv func(string) (string, bool)) string {
	home, ok := lookupEnv("HOME")
	if !ok || home == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return p
}
