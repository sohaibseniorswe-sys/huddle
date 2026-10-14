package wizard

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sohaibseniorswe-sys/huddle/internal/checks"
	"github.com/sohaibseniorswe-sys/huddle/internal/config"
)

// This file holds the wizard's background jobs. Each method returns a
// tea.Cmd: Bubble Tea runs it on another goroutine and passes the message
// it returns to Update. A job only reads the values it was given, never the
// model, so it is safe off the main loop.

// jobTimeout bounds the git and gh jobs, which normally take a second.
// The test runs of step 6 have their own limit (agents.ValidateTimeout).
const jobTimeout = time.Minute

// The starter agent files step 8 offers, shown before they are written.
var (
	//go:embed templates/AGENTS.md
	agentsTemplate string
	//go:embed templates/CLAUDE.md
	claudeTemplate string
)

// agentFiles are the files step 8 checks, in order, with their starters.
var agentFiles = []struct{ name, template string }{
	{"AGENTS.md", agentsTemplate},
	{"CLAUDE.md", claudeTemplate},
}

// folderMsg is sent when a folder check ends: by detectFolder at startup
// (detected is true) or by checkFolder after Enter on step 1.
type folderMsg struct {
	detected bool
	dir      string // the git repo's root
	err      error
}

// detectFolder looks for a git repo around the current folder.
func (m Model) detectFolder() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		wd, err := deps.Getwd()
		if err != nil {
			return folderMsg{detected: true, err: err}
		}
		dir, err := deps.TopLevel(ctx, wd)
		return folderMsg{detected: true, dir: dir, err: err}
	}
}

// checkFolder checks that path (which may start with "~") is inside a git
// repo, and finds the repo's root.
func (m Model) checkFolder(path string) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		path = expandHome(strings.TrimSpace(path), deps.Home)
		if path == "" {
			return folderMsg{err: errors.New("type the folder of the project's git clone")}
		}
		dir, err := deps.TopLevel(ctx, path)
		if err != nil {
			return folderMsg{err: fmt.Errorf("%s is not inside a git repo", path)}
		}
		return folderMsg{dir: dir}
	}
}

// expandHome replaces a leading "~" in path with home.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

// repoResult is what step 2 found.
type repoResult struct {
	repo       string // owner/name, from origin
	login      string // the gh account
	permission string // its permission on the repo: ADMIN, MAINTAIN or WRITE
}

// repoMsg is sent when checkRepo ends.
type repoMsg struct {
	dir    string // the folder that was checked
	result repoResult
	err    error
}

// checkRepo reads the origin of the repo in dir and checks it is the
// target, that gh is signed in, and that the account may read and write
// the repo, as Huddle's startup checks do.
func (m Model) checkRepo(dir string) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		fail := func(err error) tea.Msg { return repoMsg{dir: dir, err: err} }
		url, err := deps.RemoteURL(ctx, dir, "origin")
		if err != nil {
			return fail(fmt.Errorf("no origin remote: %w", err))
		}
		repo, err := checks.ParseRemoteURL(url)
		if err != nil {
			return fail(err)
		}
		if err := checks.MatchRemote(url, deps.Target); err != nil {
			return fail(fmt.Errorf("%w (run huddle --repo %s setup to set up that repo)", err, repo))
		}
		if err := checks.CheckGitHub(ctx, deps.GH, repo); err != nil {
			return fail(err)
		}
		login, err := deps.GH.User(ctx)
		if err != nil {
			return fail(fmt.Errorf("read the gh account: %w", err))
		}
		perm, err := deps.GH.Permission(ctx, repo)
		if err != nil {
			return fail(fmt.Errorf("read your permission on %s: %w", repo, err))
		}
		switch perm {
		case "ADMIN", "MAINTAIN", "WRITE":
		default:
			return fail(fmt.Errorf("%s can only %s %s; Huddle needs write access", login, strings.ToLower(perm), repo))
		}
		return repoMsg{dir: dir, result: repoResult{repo: repo, login: login, permission: perm}}
	}
}

// checkState is where one row of step 6 is.
type checkState int

const (
	checkPending checkState = iota
	checkRunning
	checkOK
	checkFailed
)

// checkRow is one row of step 6: a CLI's login, or a role's test run.
type checkRow struct {
	label  string // "Codex" or "Dev"
	what   string // "Codex · GPT-6.1 Sol · effort medium" (test runs)
	binary string // login rows: the CLI to check
	role   int    // test-run rows: the role; -1 for a login row
	key    string // test-run rows: the choice's choiceKey
	state  checkState
	detail string // the version, or the error
}

// checkMsg is sent when one row's check ends.
type checkMsg struct {
	run    int // Model.checkRun when it started
	index  int // the row
	detail string
	err    error
}

// runCheck runs row index of step 6: a login check or a test run.
func (m Model) runCheck(index int) tea.Cmd {
	deps, run, row := m.deps, m.checkRun, m.checks[index]
	var choice agentChoice
	if row.role >= 0 {
		choice = m.roleChoice(row.role)
	}
	return func() tea.Msg {
		if row.role < 0 {
			ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
			defer cancel()
			version, err := deps.CheckCLI(ctx, row.binary)
			return checkMsg{run: run, index: index, detail: version, err: err}
		}
		// agents.Validate has its own time limit.
		err := deps.Validate(context.Background(), choice.agentCLI(), choice.modelValue(), choice.effortValue())
		return checkMsg{run: run, index: index, err: err}
	}
}

// fileState is one agent file in the project.
type fileState struct {
	name   string
	exists bool
	text   string // its content, when it exists
}

// prepResult is what step 8 found.
type prepResult struct {
	created []string    // labels created
	files   []fileState // AGENTS.md and CLAUDE.md
}

// prepMsg is sent when prepare ends.
type prepMsg struct {
	result prepResult
	err    error
}

// prepare creates the labels Huddle needs that the repo is missing, and
// reads AGENTS.md and CLAUDE.md.
func (m Model) prepare() tea.Cmd {
	deps, dir, repo := m.deps, m.dir, m.repo.repo
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		var r prepResult
		var err error
		if r.created, err = checks.EnsureLabels(ctx, deps.GH, repo); err != nil {
			return prepMsg{err: err}
		}
		for _, f := range agentFiles {
			data, err := os.ReadFile(filepath.Join(dir, f.name))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				r.files = append(r.files, fileState{name: f.name})
			case err != nil:
				return prepMsg{err: err}
			default:
				r.files = append(r.files, fileState{name: f.name, exists: true, text: string(data)})
			}
		}
		return prepMsg{result: r}
	}
}

// savedMsg is sent when save ends.
type savedMsg struct {
	config config.Config
	err    error
}

// save writes the settings file, then the starter agent files chosen in
// step 8, and commits those files if asked to.
func (m Model) save() tea.Cmd {
	deps, c, dir, commit := m.deps, m.config(), m.dir, m.commit
	files := m.filesToCreate()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), jobTimeout)
		defer cancel()
		if err := c.Validate(); err != nil {
			return savedMsg{err: err} // the chips only offer valid values
		}
		if err := config.Save(deps.Home, c); err != nil {
			return savedMsg{err: err}
		}
		var names []string
		for _, f := range files {
			if err := writeNew(filepath.Join(dir, f.name), f.template); err != nil {
				return savedMsg{err: err}
			}
			names = append(names, f.name)
		}
		if commit && len(names) > 0 {
			if err := deps.Commit(ctx, dir, "Add agent files for Huddle", names...); err != nil {
				return savedMsg{err: fmt.Errorf("settings and files saved, but the commit failed: %w", err)}
			}
		}
		return savedMsg{config: c}
	}
}

// writeNew writes a file that must not exist yet: a file created since
// step 8 looked is never overwritten.
func writeNew(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(text); err != nil {
		f.Close() // the write error is the one worth reporting
		return err
	}
	return f.Close()
}

// conflictLevel is the autonomy level from which AGENTS.md rules that make
// the agents wait for a human fight the chosen level.
const conflictLevel = 6

// conflictPhrases are rules that make an agent stop and wait, which fight
// an autonomy level of conflictLevel or more. Matched without regard to
// case.
var conflictPhrases = []string{
	"never push", "do not push", "don't push",
	"never merge", "do not merge", "don't merge",
	"ask before", "wait for approval",
}

// Conflict is a line of an agent file with a rule that fights the
// autonomy level.
type Conflict struct {
	Line   int    // 1-based
	Text   string // the line, trimmed
	Phrase string // the phrase found
}

// Conflicts returns the lines of text that fight autonomy level: none
// below conflictLevel. A line with several phrases is reported once.
func Conflicts(text string, level int) []Conflict {
	if level < conflictLevel {
		return nil
	}
	var out []Conflict
	for i, line := range strings.Split(text, "\n") {
		lower := strings.ToLower(line)
		for _, p := range conflictPhrases {
			if strings.Contains(lower, p) {
				out = append(out, Conflict{Line: i + 1, Text: strings.TrimSpace(line), Phrase: p})
				break
			}
		}
	}
	return out
}
