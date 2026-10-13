# Huddle MVP: Build Plan

> **For Claude Code.** This file is the spec for the Huddle MVP. Work through it one milestone per session. At the end of each milestone: `go build ./...` and `go test ./...` must pass, then stop and summarise what you built, what you verified, and anything you were unsure about. Do not start work from a later milestone early. If something here is ambiguous or turns out to be wrong in practice (for example a CLI flag), stop and ask rather than guess.
>
> Optional first step: create the five milestones and their issues on GitHub from this file using `gh`, in `sohaibseniorswe-sys/huddle`.
>
> **Design change after Milestone 2.** Milestones 1 and 2 were built for an earlier design (Huddle-owned control issue, verdict JSON, turn limits, timeouts, restricted agents). From Milestone 3 on, this file describes the new design. Code from Milestones 1 and 2 that the new design no longer needs is removed in Milestones 3 and 4.

---

## 1. What Huddle is

Huddle is a small Go program that lets two AI coding agents complete a GitHub milestone from design to delivery in one go, with no human involvement unless the world is on fire.

- **Dev agent:** Codex CLI. Works through the whole milestone, issue after issue: branch, commit, push, open and merge PRs, close issues.
- **PM agent:** Claude Code. Reads what the Dev reported, makes decisions, and hands the Dev a clear next prompt. Writes the final report when the milestone is done.
- **Huddle:** starts the right agent when it is that agent's turn, and does nothing else. It never judges the agents' work.

The **control issue** is the channel between the agents. They read it, post their own comments on it, and set its `state:*` label themselves at the end of each turn. Huddle watches the label.

**Philosophy.** Huddle adds no limits, no timeouts and no restrictions. Agents have full permissions. The target is a private repo, and everything can be reverted.

Planning is done before Huddle runs. The milestone, its issues, the control issue and `AGENTS.md` already exist in the target repo.

## 2. Repositories

| Repo | Role |
|------|------|
| `sohaibseniorswe-sys/huddle` | This repo. Huddle's source code. Go module `github.com/sohaibseniorswe-sys/huddle`. |
| `sohaibseniorswe-sys/palantir` | The real target. The **default**, hardcoded in Huddle. |
| `sohaibseniorswe-sys/huddle-sandbox` | A throwaway target for development and early runs (section 4b). Selected with `--repo`. |

Huddle always runs from inside a local clone of its target repo. The target is `sohaibseniorswe-sys/palantir` unless `--repo owner/name` is given. `--repo` exists so that early, buggy runs create branches, PRs and comments in the sandbox, never in palantir. Palantir only sees Huddle once the full loop works in the sandbox.

Platform: Linux (WSL). The author keeps projects in `~/projects`. Single foreground process, for example:

```bash
cd ~/projects/huddle-sandbox && huddle --repo sohaibseniorswe-sys/huddle-sandbox
cd ~/projects/palantir && huddle
```

## 3. Hard constraints

1. **No API billing, ever.** Refuse to start if `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` is set in the environment. Also remove both from the environment of every child process. Never add any fallback that uses an API key.
2. **One agent at a time.** Never run two agent processes concurrently.
3. **No limits.** No turn limits, no per-issue limits, no agent timeouts, no allow or deny lists. Huddle never stops the loop on its own; it stops only on `state:done` or Ctrl+C.
4. **Agent commands are hardcoded** (section 6). No config file in the MVP. The target repo is hardcoded to palantir, with `--repo` as the only override, for testing.

## 4. Prerequisites in the target repo (not built by Huddle)

These must exist in whichever repo Huddle targets: the sandbox (section 4b) and later palantir. Huddle checks for them at startup.

- `AGENTS.md` (committed): how to work in this repo (branching, tests, PRs, merging). The agents follow it "as they normally would"; it must not forbid what the Dev role needs (pushing branches, merging PRs, closing issues).
- `CLAUDE.md` (committed): one line, `Read and follow AGENTS.md.` (plus `Session.md` if the repo uses it).
- `Session.md` (optional, **gitignored**): shared scratch notes. Huddle creates it empty if missing, so it does not dirty the tree. The operating principles tell agents to read it "if present".
- Exactly one open issue labelled `control` (in palantir this is #58), assigned to the milestone being worked on. Its body states the goal; there is no required format. Its comments become the full history of the run.

To start a run, set `state:dev-ready` on the control issue (or `state:pm-ready` to have the PM write the first Dev prompt).

## 4b. Sandbox repo

`sohaibseniorswe-sys/huddle-sandbox` is a tiny, disposable Go repo set up like palantir, so the full loop (including CI runs) can be exercised without touching real work. It was created in issue 1.4 with the commands below, and its agent files are updated for the new design in issue 4.3.

**Create the repo and its code**

```bash
cd ~/projects
gh repo create sohaibseniorswe-sys/huddle-sandbox --private --add-readme --clone
cd huddle-sandbox
go mod init github.com/sohaibseniorswe-sys/huddle-sandbox

cat > strutil.go <<'EOF'
package sandbox

import "strings"

// Upper returns s in upper case.
func Upper(s string) string { return strings.ToUpper(s) }
EOF

cat > strutil_test.go <<'EOF'
package sandbox

import "testing"

func TestUpper(t *testing.T) {
	if got := Upper("huddle"); got != "HUDDLE" {
		t.Fatalf("Upper() = %q, want %q", got, "HUDDLE")
	}
}
EOF

mkdir -p .github/workflows
cat > .github/workflows/ci.yml <<'EOF'
name: ci
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go test ./...
EOF
```

**Agent files (new design)**

```bash
cat > AGENTS.md <<'EOF'
# Agent rules

- Work on a branch per issue (agent/issue-<n>), open a PR that closes the issue, and merge it once `go test ./...` and CI pass.
- Run `go test ./...` before every push.
- Keep main green.
- Huddle and the control issue: follow the operating principles in your launch prompt.
EOF
echo "Read and follow AGENTS.md." > CLAUDE.md
echo "Session.md" > .gitignore
go test ./...
git add -A && git commit -m "Sandbox agent files for the new Huddle design" && git push
```

**Milestone, labels, work issues and control issue**

```bash
S=sohaibseniorswe-sys/huddle-sandbox
gh api repos/$S/milestones -f title="Sandbox M1" > /dev/null
gh label create control --repo $S --color 5319e7 --force
gh label create "state:dev-ready" --repo $S --color 7057ff --force

I1=$(gh issue create --repo $S --milestone "Sandbox M1" \
  --title "Add Reverse(s string) string" \
  --body "Add a Reverse function that reverses a string by runes, with a doc comment.
Acceptance: table-driven tests covering an empty string, an ASCII string and a string containing an emoji; go test passes." \
  | grep -o '[0-9]*$')

I2=$(gh issue create --repo $S --milestone "Sandbox M1" \
  --title "Document Upper and Reverse in the README" \
  --body "Add a Usage section to README.md with one example each for Upper and Reverse.
Acceptance: README has a Usage section covering both functions." \
  | grep -o '[0-9]*$')

gh issue create --repo $S --milestone "Sandbox M1" --label control \
  --title "Sandbox Control" --body "## Goal
Complete milestone Sandbox M1 (#$I1, #$I2) end to end."
```

To start a run, add `state:dev-ready` to the sandbox control issue.

**Resetting the sandbox between runs**

Revert merged changes on `main` (or reset it to the setup commit and force-push), close open PRs and delete `agent/*` branches, reopen the work issues, then close the control issue and create a fresh one with the command above. Delete Huddle's local session file for the old control issue (section 8.4). If the sandbox gets too messy, delete and recreate it: `gh repo delete sohaibseniorswe-sys/huddle-sandbox --yes` (needs `gh auth refresh -s delete_repo` once).

## 5. Labels

Huddle creates these on the target repo if missing (idempotent):

| Label | Set by | Meaning |
|-------|--------|---------|
| `control` | human | Marks the control issue |
| `state:dev-ready` | agents (or human to start) | Codex's turn |
| `state:dev-working` | Huddle | Codex is running (visibility only) |
| `state:pm-ready` | agents (or human) | PM's turn |
| `state:pm-working` | Huddle | PM is running (visibility only) |
| `state:human-required` | PM, or Huddle for account and credit errors (8.6) | Waiting for the human |
| `state:done` | PM | Milestone finished |

Agents set the label with `gh issue edit`. They may forget to remove the old one, so Huddle normalises: after each run it removes its own `*-working` label, and whenever it sets a label it removes all other `state:*` labels. If several non-working `state:*` labels are present when Huddle reads the issue, it takes the first in this order and removes the rest: `human-required`, `done`, `pm-ready`, `dev-ready`.

## 6. Agent commands (hardcoded)

```
Dev, first run:  codex exec --strict-config --ignore-user-config \
                   -c sandbox_mode="danger-full-access" -c approval_policy="never" -
Dev, resume:     codex exec resume <session-id> ... -
PM,  first run:  claude -p --output-format json --dangerously-skip-permissions
PM,  resume:     claude -p --output-format json --dangerously-skip-permissions --resume <session-id>
```

- Both run with the working directory set to the repo root, with the prompt on stdin.
- No `--allowedTools`, `--disallowedTools`, `--add-dir`, `--permission-mode` or other allow and deny lists.
- **Verify every flag in Milestone 3** against `codex exec --help`, `codex exec resume --help` and `claude --help` for the installed versions (in particular: which options `codex exec resume` accepts, whether it reads the prompt from stdin, and this Claude Code version's equivalent of `--dangerously-skip-permissions`). Record versions and final commands in `README.md`.
- **Session ids.** Claude Code's JSON envelope has a `session_id` field. For Codex, find the documented way to get the id of the session `codex exec` just created (for example its header line or a `--json` event) and verify it. Never use "resume the last session" shortcuts: another Codex session in the same directory would be picked up.
- **Final output.** Huddle needs each agent's final message for the anti-stall comment (section 8.3): Claude Code's envelope `result` field; for Codex, its last-message output option if one exists (verify), otherwise the tail of its output.

## 7. Startup sequence

```
$ huddle
✔ git repo ~/projects/palantir, remote sohaibseniorswe-sys/palantir
✔ AGENTS.md, CLAUDE.md and Session.md present
✔ gh logged in, repo access ok, labels ready
✔ codex and claude installed with subscription logins, no API keys in env
✔ control issue #58 "Palantir V3 Control", milestone "V3", state: dev-ready
Continue? [Y/n]
```

As built in Milestone 1, minus the clean-tree check, which is removed completely: after an interrupted run the tree is normally dirty, and Huddle must be able to resume the agent (section 8.1). Checks run in this order. The billing guard runs first, so nothing touches git, `gh` or the agent CLIs while an API key is set.

1. `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` are not set (even to an empty value).
2. Current directory is inside a git repo, and its `origin` remote is the target repo: `sohaibseniorswe-sys/palantir`, or the `--repo` value if given (accept HTTPS and SSH URL forms, with or without `.git`; the host must be `github.com`, so SSH host aliases are rejected). When `--repo` is used, print a clear line such as `target: sohaibseniorswe-sys/huddle-sandbox (override)`.
3. `AGENTS.md` and `CLAUDE.md` exist; create an empty `Session.md` if missing.
4. `gh auth status` succeeds; `gh repo view <target>` succeeds.
5. Ensure all labels exist (this also proves write access).
6. `codex` and `claude` are on `PATH` and logged in with a subscription (see README.md).
7. Exactly one open issue has the `control` label. It must have a milestone.
8. Print the summary and ask `Continue? [Y/n]`. A `--yes` flag skips the prompt.

Any failed check prints a clear reason and exits non-zero.

## 8. The watch loop

### 8.1 What Huddle does for each label

Huddle reads the control issue's `state:*` label (normalised, section 5) every 30 seconds, and immediately after an agent exits.

| Label | Action |
|-------|--------|
| `state:dev-ready` | Run a Dev turn (8.2) |
| `state:pm-ready` | Run a PM turn (8.2) |
| `state:human-required` | Print the latest comment on the control issue (once per change), then keep polling until a ready label is set |
| `state:done` | Print the PM's final report (the latest comment on the control issue) and exit 0 |
| `state:dev-working` / `state:pm-working` | A previous Huddle run was interrupted. Treat it as `dev-ready` / `pm-ready` and start that agent again (resuming its session) |
| no `state:*` label | Print "add state:dev-ready to start" once, then keep polling |

### 8.2 A turn

1. Set `state:dev-working` or `state:pm-working`.
2. Build the launch prompt (section 9).
3. Run the agent (section 6), resuming its saved session if there is one. Stream output to the terminal and save it to `~/.huddle/logs/<timestamp>-turn<N>-<role>.log`, where N is a counter local to this Huddle process (used for log names only). No timeout.
4. After it exits, kill its process group (as the runner already does), save its session id (8.4), remove Huddle's `*-working` label, and read the label again.
5. If the agent set a label, go back to 8.1. If its output shows a usage limit, wait and resume; if it shows an account or credit error, escalate to the human (8.6). Otherwise apply anti-stall (8.3).

### 8.3 Anti-stall

If an agent exits without setting a new `state:*` label (whatever its exit code), Huddle:

1. Posts a comment on the control issue with the agent's final output (section 6), its exit code, and the marker `<!-- huddle:auto-handoff role=dev|pm -->`.
2. Hands the turn to the other agent: after Codex, set `state:pm-ready`; after the PM, set `state:dev-ready`.

Huddle never stops the loop on its own.

### 8.4 Sessions

- Huddle keeps one session id per agent per control issue in `~/.huddle/sessions/<owner>_<repo>_<control-issue>.json` (`{"dev": "...", "pm": "..."}`), so sessions survive a restart of Huddle.
- When an agent is started again, its saved session is resumed. **Fallback:** if a resumed run exits non-zero without setting a new label, Huddle forgets that session and immediately runs the same turn once more as a fresh session. Only if that run also leaves the label unchanged does anti-stall apply.
- A fresh session's id replaces the saved one.

### 8.5 Ctrl+C and `--once`

- **Ctrl+C:** kill the running agent's process group, put back the ready label of the agent that was interrupted (so the next `huddle` resumes it), exit 130. This also applies while Huddle is waiting for a usage reset (8.6). While no agent is running, just exit.
- **`--once`:** run a single turn (including its anti-stall, resume fallback or usage-limit wait), then exit. Used for the first sandbox runs.

### 8.6 Usage limits

A subscription usage limit is not a reason to hand the turn over: the other agent cannot finish the first one's work.

1. **Detect.** After a run that left the label unchanged, check the agent's output (log, final message, and for Claude Code the JSON envelope) for that CLI's usage-limit, account and credit messages (listed in the README). The check comes before the resume fallback and anti-stall: a limited or escalated run never forgets its session and never hands over. Each message falls into one of three cases:
   - **Wait** (steps 2–4): any message that gives a reset time, and plain usage limits without one (session, weekly, model-specific and similar plan limits).
   - **Escalate** (step 5): account-setup errors that no wait will fix (for example a seat type without usage, usage disabled by an admin, a plan that does not include the CLI, a model that requires credits), and spend-cap, out-of-credits and spend-limit messages that give no reset time. These need money or account changes only the human can make.
   - If one run's output shows several, a message with a reset time wins (it will clear by itself), then an escalation, then a plain usage limit.
2. **Comment once.** Post one comment on the control issue: which agent hit the limit, the reset time if known, and the marker `<!-- huddle:usage-wait role=dev|pm -->`. Do not post again for retries within the same wait. Keep the `*-working` label during the wait (the turn is not over), and print the wait on the terminal.
3. **Wait.** If the CLI printed a reset time, sleep until then plus one minute. Otherwise (or if the time cannot be parsed or is in the past) retry every 15 minutes.
4. **Resume.** Run the same agent again, resuming its session, with the normal launch prompt. If it hits the limit again, keep waiting without a new comment. Once a run gets past the limit, continue as in 8.2 step 5. If a retry shows an escalation message instead, escalate (step 5).
5. **Escalate.** Post one comment on the control issue: which agent was stopped, the CLI's exact message as the reason, that only the human can fix it, and the marker `<!-- huddle:escalate role=dev|pm -->`. Then set `state:human-required` (which removes the `*-working` label) and carry on as in 8.1: print the comment, keep polling. Keep the agent's session; when the human sets a ready label again, that agent resumes it. This is the "world on fire" case of the operating principles, and the only time Huddle itself sets `state:human-required`.

**The real messages.** Find the exact usage-limit messages each installed CLI prints, from its docs or source (Codex is open source; for Claude Code use its docs, changelog and observed output), including how and in what format and time zone the reset time is given. Record them in the README. Match on those messages specifically, not on any output containing "limit", so a test failure that mentions limits is not mistaken for one.

## 9. Launch prompt

Every launch, fresh or resumed, sends:

```
<shared section>

<the agent's role section>

It's your turn. Read control issue #N and continue.
```

The sections are the operating principles below, embedded with `go:embed` **exactly as written**, as three files in `internal/prompts/`: `shared.md`, `dev.md`, `pm.md`. `{{milestone}}` and `{{control_issue}}` are filled with plain string replacement (not `text/template`). `N` is the control issue number.

### Operating principles (embed exactly)

```
HOW HUDDLE WORKS
You are one of two agents completing milestone "{{milestone}}" without the human. Huddle is the program that starts you; it does nothing else.
- Control issue #{{control_issue}} is the shared channel. Its comments are the full history: every status update, question and decision.
- Its label says whose turn it is: state:dev-ready (Codex), state:pm-ready (PM), state:human-required (the human), state:done (finished). Huddle watches the label and starts the right agent.
- At the start of every turn, read: the control issue and all its comments, the milestone's issues (gh issue list --milestone "{{milestone}}" --state all), AGENTS.md, Session.md if present, and recent git history.
- At the end of your turn, post one comment on the control issue, then set the label for whoever goes next. That comment is how the other agent knows what happened, so make it complete:
    Status: Progress | Decision needed | Error | Completed
    Done:   what you did (issues closed, PRs, key commits)
    Open:   what is left, and any question or blocker
    Next:   what should happen now
- This is a private repo and everything can be reverted. The goal is to keep moving until the milestone is complete.
```

```
YOUR ROLE: DEV (Codex)
Your job is to complete the whole milestone, issue after issue, until every issue is closed. Work the way AGENTS.md describes, as you normally would: branch, commit, push, open and merge PRs, close issues.
Make reasonable decisions yourself and note any assumption in the issue or PR. Do not stop to ask for review, approval or confirmation.
If something blocks one issue, leave a note on it and carry on with the rest. Hand over to the PM (state:pm-ready) only when you cannot make further progress on the milestone, or when everything is done, with all open questions and blockers in that one comment.
Write good, tested code. Don't knowingly break main.
```

```
YOUR ROLE: PM (Claude Code)
Your job is to keep the milestone moving so the human is never disturbed. You are not a gatekeeper.
Read the latest Dev comment and the project state, decide, then post Codex a clear, actionable prompt and set state:dev-ready.
Prefer a sensible decision over a question. If a choice can be changed later, make it, record it, and move on. Never add restrictions like "don't push" or "ask before committing".
If an issue genuinely needs the human (for example, manual testing), tell Codex to leave it open with a note and finish everything else.
When the Dev reports Completed, check the milestone. If every issue is closed, post a final report (what was built, the exact commands to run and test it, a suggested next milestone) and set state:done. If only human-only issues remain, post the same report, list exactly what the human must do, and set state:human-required.
Set state:human-required at any other time only when the world is on fire: you need credentials, money or access only the human has, or you and the Dev have truly run out of ways forward.
```

## 10. Code layout

```
huddle/
├── cmd/huddle/main.go        flags, startup, signal handling
├── internal/
│   ├── checks/               startup checks
│   ├── gh/                   gh wrapper behind an interface (fakeable in tests)
│   ├── gitx/                 git helpers
│   ├── runner/               run an agent: process group, env stripping, logging; agent commands
│   ├── session/              per-control-issue session ids on disk
│   ├── loop/                 watch loop, turns, anti-stall
│   └── prompts/              go:embed operating principles (shared.md, dev.md, pm.md)
├── README.md
└── MVP_PLAN.md               this file
```

Shell out to `git`, `gh`, `codex` and `claude`. Prefer the standard library; avoid dependencies unless they clearly earn their place.

---

## Milestones

### Milestone 1: Skeleton and startup checks (done)

Built for the earlier design; kept as is. Startup checks, the API-key guard, `--repo`, `--yes`, `--version`, the `gh` wrapper with a fake, idempotent labels, the sandbox repo.

### Milestone 2: Agent runner (done)

Built for the earlier design. The runner (streaming, logging, tail capture, process-group cleanup, env stripping) is kept. Its timeout and the restricted agent commands are replaced in Milestone 3.

### Milestone 3: Agent commands, sessions, control issue and prompts (2–3 h)

**3.1 Runner without limits**
- Remove the runner's default timeout: an unset timeout means none. Keep streaming, logging, tail capture, process-group cleanup after every run and on cancel, and env stripping.
- Acceptance: existing runner tests still pass, minus the timeout tests; a test shows a run with no timeout set is not killed early.

**3.2 Agent commands with full permissions and resume**
- Replace the Dev and PM command builders with section 6: Codex with `danger-full-access` and `approval_policy="never"`, Claude Code with `--dangerously-skip-permissions` (or this version's equivalent). Remove `PMAllowedTools`, `PMDisallowedTools`, `--add-dir` and `GoCacheDirs`. Add a resume form of each command.
- Extract the session id and the final message from each agent's output (section 6).
- Verify every flag in `--help` for the installed versions; update the README's agent-command section (versions, final commands, how the session id and final message are obtained).
- Acceptance: unit tests for both builders (fresh and resume) and for session-id and final-message extraction from recorded real output. The smoke test (`HUDDLE_SMOKE`) runs each CLI fresh, then resumes the same session and asks it to repeat a word from the first run.

**3.3 Session store**
- `internal/session`: load and save `{"dev": "...", "pm": "..."}` per `<owner>_<repo>_<control-issue>` under `~/.huddle/sessions/`, with forget. A missing or unreadable file means no sessions.
- Acceptance: tests for round trip, missing file, corrupt file and forget.

**3.4 Control issue operations**
- In the `gh` wrapper and its fake: read the control issue's labels, set a state (removing all other `state:*` labels), remove one label, read the latest comment, post a comment. Normalisation from section 5.
- Acceptance: tests with the fake for each operation, including no comments, no `state:*` label, several `state:*` labels (priority order), and an agent-set label next to a leftover `*-working` label.

**3.5 Usage-limit detection**
- Section 8.6: find each CLI's real usage-limit messages (docs or source) and record them in the README; write a detector that returns whether a run hit a usage limit and the reset time if the output gives one, or whether it shows an account or credit error to escalate (with the CLI's exact message).
- Acceptance: table-driven tests with fake outputs for each CLI: limit with a reset time, limit without one, a reset time in an unexpected format (detected, no time), an account or credit error (escalate, with its exact message), normal output, and output that mentions "limit" in another context (not detected).

**3.6 Operating principles**
- `internal/prompts`: embed `shared.md`, `dev.md`, `pm.md` exactly as in section 9; render the launch prompt for each role.
- Acceptance: golden-file tests for the rendered Dev and PM prompts; a test that the embedded text matches section 9 apart from the two placeholders and that no `{{` is left after rendering.

### Milestone 4: Watch loop and wiring (2–3 h)

**4.1 Watch loop**
- `internal/loop`: section 8 in full: label dispatch, 30 s polling and the immediate re-read after a run, `*-working` labels, anti-stall, resume with fresh-session fallback, usage-limit wait (8.6), human-required printing, `done` printing the final report, interrupted-run recovery, Ctrl+C, `--once`. Runner, `gh`, sessions and clock behind interfaces.
- Acceptance: tests with a fake runner, fake `gh` and fake clock cover: dev sets `pm-ready` → PM runs; PM sets `done` → report printed, exit 0; agent exits without changing the label → handoff comment with its final output and the other agent's ready label (for both roles, and for a non-zero exit); resume fails → fresh run of the same turn → session replaced; usage limit with a reset time → one wait comment, sleep until the reset, same agent resumed, no handoff; usage limit without a reset time → retries every 15 minutes with still only one comment; account or credit error → one escalate comment with the CLI's exact message, `state:human-required`, session kept, no handoff; Ctrl+C during a usage wait → ready label restored; `human-required` → latest comment printed once, polling continues, a ready label resumes the loop; `dev-working` at startup → Codex resumed; no label → polling; Ctrl+C during a run → process killed, ready label restored; `--once` stops after one turn.

**4.2 Wiring and cleanup**
- Connect the startup checks to the loop in `main.go`. Remove what the new design dropped: the clean-tree check (startup check and `CheckCleanTree`), the `verdict` package, any facts, turn-limit or merge code, and the old preamble plans.
- Update the README for the new design: usage, flags, labels and who sets them, the anti-stall rule, sessions, Ctrl+C.
- Acceptance: `go build ./...`, `go test ./...` and `go vet ./...` pass; `grep` finds no `verdict`, `Timeout: 30`, `allowedTools` or `disallowedTools` left in the code.

**4.3 Sandbox ready for the new design**
- Update the sandbox's `AGENTS.md` and `CLAUDE.md` and recreate its control issue as in section 4b. Reset `main` and the work issues if needed.
- Acceptance: `huddle --repo sohaibseniorswe-sys/huddle-sandbox` in `~/projects/huddle-sandbox` passes all startup checks and waits for a label.

### Milestone 5: Sandbox runs, then pilot on palantir #58 (4–6 h)

Everything in 5.1 and 5.2 runs against the sandbox with `--repo sohaibseniorswe-sys/huddle-sandbox`. Palantir is not touched until 5.3. Reset the sandbox (section 4b) between attempts as needed.

**5.1 Single turns in the sandbox**
- With `--once`, run one Dev turn and inspect the control-issue comment, label, branches and PRs; then one PM turn and inspect its comment and label. Then exercise an anti-stall (post a control-issue comment asking the next agent to end its turn without changing the label) and a resume (run `--once` again for the same agent and check it continues its session). Fix what breaks.

**5.2 Full loop in the sandbox**
- Run unattended from `state:dev-ready` until the sandbox milestone reaches `state:done`, with no human action in between.
- Acceptance: both sandbox issues closed by merged PRs, CI green on `main`, the PM's final report on the control issue, no manual copy-pasting and no API charges.

**5.3 Pilot on palantir #58**
- Only after 5.2 passes: run `huddle` (no `--repo`) in `~/projects/palantir` on Control #58, starting from `state:dev-ready`.
- Acceptance: the milestone reaches `state:done` (or `state:human-required` with only human-only issues left) without manual copy-pasting and without API charges; the control issue's comment history shows the whole run; findings and fixes are written to `docs/PILOT.md`.

**5.4 README**
- Install, prerequisites (section 4), sandbox setup and reset (section 4b), usage, flags, what each label means and who sets it, troubleshooting for everything hit during the sandbox runs and the pilot.

---

## Out of scope for the MVP

Config files, a SQLite checkpoint, a lock file, background service, push notifications, a reviewer agent, multiple control issues, a separate GitHub account for agents. These are in the full design and come after the pilot passes.

Deliberately absent, not deferred: the clean-tree check, turn limits, per-issue limits, agent timeouts, permission allow and deny lists, PM verdict parsing, and any check by Huddle of the agents' work.
