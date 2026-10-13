// Command huddle runs a Dev agent (Codex) and a PM agent (Claude Code) in
// turns through a GitHub milestone. See MVP_PLAN.md and docs/v3.
//
// After the startup checks it shows the run screen (package app), or with
// --plain prints the loop's lines to stdout as Huddle v1 did.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sohaibseniorswe-sys/huddle/internal/app"
	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/gh"
	"github.com/sohaibseniorswe-sys/huddle/internal/gitx"
	"github.com/sohaibseniorswe-sys/huddle/internal/loop"
	"github.com/sohaibseniorswe-sys/huddle/internal/runner"
	"github.com/sohaibseniorswe-sys/huddle/internal/session"
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
	// dir. stream receives their raw output (nil for the run screen).
	runAgents func(dir, home string, stream io.Writer) loop.Agents
	clock     loop.Clock
	// runUI shows the run screen while l runs and returns l's exit code.
	runUI func(ctx context.Context, cfg app.Config, l *loop.Loop, stderr io.Writer) int
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
		runAgents: func(dir, home string, stream io.Writer) loop.Agents {
			return loop.CLIAgents{Dir: dir, Home: home, Stream: stream}
		},
		clock: loop.RealClock{},
		runUI: app.Run,
	}))
}

type options struct {
	version bool
	yes     bool
	once    bool
	plain   bool
	repo    string
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	var o options
	fs := flag.NewFlagSet("huddle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.BoolVar(&o.version, "version", false, "print the version and exit")
	fs.BoolVar(&o.yes, "yes", false, "skip the confirmation prompt")
	fs.BoolVar(&o.once, "once", false, "run a single turn, then stop (with --plain: exit)")
	fs.BoolVar(&o.plain, "plain", false, "print plain lines to stdout instead of the full-screen UI (as v1 did)")
	fs.StringVar(&o.repo, "repo", checks.DefaultRepo, "target repo as owner/name (for testing against the sandbox)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: huddle [flags]\n\nRun from inside a local clone of the target repo.\n\nFlags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if err := checks.ValidateRepo(o.repo); err != nil {
		return o, fmt.Errorf("--repo: %w", err)
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

	// Check 2: inside a git repo whose origin is the target.
	wd, err := d.getwd()
	if err != nil {
		return fail(err)
	}
	root, err := gitx.TopLevel(ctx, wd)
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

	// Check 6: agent CLIs with subscription logins.
	if err := d.agents.Check(ctx); err != nil {
		return fail(err)
	}
	ok("codex and claude installed with subscription logins, no API keys in env")

	// Check 7: the control issue.
	control, err := checks.FindControlIssue(ctx, d.gh, target)
	if err != nil {
		return fail(err)
	}
	state := "none"
	if s := checks.States(control); len(s) > 0 {
		state = strings.Join(s, ", ")
	}
	ok("control issue #%d %q, milestone %q, state: %s", control.Number, control.Title, control.Milestone, state)

	// Check 8: confirmation.
	if !opts.yes && !checks.Confirm(d.stdin, d.stdout) {
		fmt.Fprintln(d.stdout, "aborted")
		return 1
	}

	home, hasHome := d.lookupEnv("HOME")
	if !hasHome || home == "" {
		return fail(errors.New("HOME is not set"))
	}
	l := &loop.Loop{
		Control:   gh.Control{Client: d.gh, Repo: target, Number: control.Number},
		Milestone: control.Milestone,
		Sessions:  session.New(home, target, control.Number),
		Clock:     d.clock,
		Once:      opts.once,
	}

	// --plain: the agents' raw output and Huddle's lines go to stdout.
	if opts.plain {
		l.Agents = d.runAgents(root, home, d.stdout)
		l.Report = loop.PlainReporter{Out: d.stdout}
		l.Controls = &loop.Controls{}
		return l.Run(ctx)
	}

	// The run screen: nothing may write to the terminal but the screen.
	l.Agents = d.runAgents(root, home, nil)
	return d.runUI(ctx, app.Config{
		Repo:      target,
		Milestone: control.Milestone,
		Control:   l.Control,
		LogDir:    runner.LogDir(home),
		RepoDir:   root,
		Now:       time.Now,
	}, l, d.stderr)
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
