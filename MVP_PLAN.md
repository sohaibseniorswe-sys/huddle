# Huddle MVP: Build Plan

> **For Claude Code.** This file is the spec for the Huddle MVP. Work through it one milestone per session. At the end of each milestone: `go build ./...` and `go test ./...` must pass, then stop and summarise what you built, what you verified, and anything you were unsure about. Do not start work from a later milestone early. If something here is ambiguous or turns out to be wrong in practice (for example a CLI flag), stop and ask rather than guess.
>
> Optional first step: create the five milestones and their issues on GitHub from this file using `gh`, in `sohaibseniorswe-sys/huddle`.

---

## 1. What Huddle is

Huddle is a small Go program that lets two AI coding agents work through a GitHub milestone in turns, with the human stepping in only when the agents escalate.

- **Dev agent:** Codex CLI. Implements one issue per turn on a branch and opens a PR.
- **PM agent:** Claude Code. Reviews what the Dev did, then decides: continue with a new prompt, the milestone is done, or a human is needed.
- **Huddle:** keeps the schedule and the record. It never judges the agents' work. It runs one agent at a time, passes control between them, and moves the label on a single "control issue".

Planning is done before Huddle runs. The milestone, its issues, the control issue, `AGENTS.md`, `CLAUDE.md` and `Session.md` already exist in the target repo.

## 2. Repositories

| Repo | Role |
|------|------|
| `sohaibseniorswe-sys/huddle` | This repo. Huddle's source code. Go module `github.com/sohaibseniorswe-sys/huddle`. |
| `sohaibseniorswe-sys/palantir` | The target, **hardcoded** in Huddle. Huddle is run from inside a local clone of it. |

Platform: Linux (WSL). Single foreground process: `cd ~/code/palantir && huddle`.

## 3. Hard constraints

1. **No API billing, ever.** Refuse to start if `OPENAI_API_KEY` or `ANTHROPIC_API_KEY` is set in the environment. Also remove both from the environment of every child process. Never add any fallback that uses an API key.
2. **Huddle alone owns the control issue.** Only Huddle changes its `state:*` label and posts on it. Agents work in the work plane (branches, PRs, comments on work issues).
3. **Never touch `main` directly.** Agents work on `agent/issue-<n>` branches. The human merges PRs.
4. **One agent at a time.** Never run two agent processes concurrently.
5. **Agent commands are hardcoded** (section 6). No config file in the MVP.

## 4. Prerequisites in `palantir` (not built by Huddle)

The human sets these up before running Huddle. Huddle checks for them at startup.

- `AGENTS.md` (committed): rules for both agents, including:
  - Dev works on branch `agent/issue-<n>`, one PR per issue, never pushes to `main`.
  - Dev runs the tests before finishing.
  - Both agents read `Session.md` at the start and update it at the end of every turn.
  - The PM only edits `Session.md`. It never edits code, labels, or the control issue.
- `CLAUDE.md` (committed): one line, `Read and follow AGENTS.md and Session.md.`
- `Session.md`: **gitignored**, so both agents share one copy on disk regardless of the checked-out branch. Huddle creates it empty if missing.
- `.gitignore` contains `Session.md`.
- Exactly one open issue labelled `control` (the control issue, #58). Its body contains:

  ```markdown
  ## First Dev task
  Target issue: #59
  Prompt:
  <the first prompt for the Dev agent>
  ```

- The control issue is assigned to the milestone being worked on.

## 5. Labels

Huddle creates these on `palantir` if missing (idempotent):

| Label | Meaning |
|-------|---------|
| `control` | Marks the control issue |
| `state:dev-ready` | Waiting for the Dev agent |
| `state:dev-working` | Dev agent running |
| `state:pm-ready` | Waiting for the PM agent |
| `state:pm-working` | PM agent running |
| `state:human-required` | Waiting for the human |
| `state:done` | Milestone finished |

The control issue has exactly one `state:*` label at a time. Setting a state removes any other `state:*` label.

## 6. Agent commands (hardcoded)

```
Dev:  codex exec --full-auto "<prompt>"
PM:   claude -p "<prompt>" --output-format json --permission-mode acceptEdits \
        --allowedTools "Bash(gh issue view:*)" "Bash(gh pr view:*)" \
                       "Bash(gh pr diff:*)" "Bash(gh pr checks:*)"
```

- Both run with the working directory set to the repo root.
- These flags reflect the CLIs as known at design time. **In Milestone 2, verify every flag against `codex exec --help` and `claude --help`** for the installed versions, adjust if needed, and record the versions and final commands in `README.md`.
- If a prompt is too long for a command-line argument, pass it on stdin if the CLI supports that.
- With `--output-format json`, Claude Code wraps its answer in a JSON envelope. Huddle must first extract the answer text (the `result` field, verify the exact field name) and only then parse the verdict from that text.

## 7. Startup sequence

```
$ huddle
✔ git repo ~/code/palantir, remote sohaibseniorswe-sys/palantir
✔ working tree clean
✔ gh logged in, repo access ok, labels ready
✔ codex and claude installed, no API keys in env
✔ control issue #58 "Palantir V3 Control", milestone "V3", state: dev-ready
Continue? [Y/n]
```

1. Current directory is inside a git repo, and its `origin` remote is `sohaibseniorswe-sys/palantir` (accept HTTPS and SSH URL forms, with or without `.git`).
2. Working tree is clean (`git status --porcelain` is empty, ignoring `Session.md`).
3. `gh auth status` succeeds; `gh repo view sohaibseniorswe-sys/palantir` succeeds.
4. Ensure all labels exist (this also proves write access).
5. `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` are not set.
6. `codex` and `claude` are on `PATH`. If either CLI offers a reliable non-billable way to show its login method, use it and require a subscription login; if not, document that in the README and rely on check 5.
7. Exactly one open issue has the `control` label. It must have a milestone.
8. `AGENTS.md` and `CLAUDE.md` exist; create an empty `Session.md` if missing.
9. Print the summary and ask `Continue? [Y/n]`. A `--yes` flag skips the prompt.

Any failed check prints a clear reason and exits non-zero.

## 8. The turn loop

Huddle reads the control issue's `state:*` label and acts on it, repeating until it reaches `done`.

| Label at loop start | Action |
|---------------------|--------|
| `state:dev-ready` | Run a Dev turn |
| `state:pm-ready` | Run a PM turn |
| `state:human-required` | Print the reason, then poll every 60 s until the human changes the label |
| `state:done` | Print "milestone done" and exit 0 |
| `state:dev-working` or `state:pm-working` | A previous run was interrupted. Set `state:human-required` with the note "previous run interrupted", then continue the loop |
| No `state:*` label | Print "add state:dev-ready to start" and poll every 60 s |

### Dev turn

1. Check the turn limit (below). If reached, set `state:human-required` ("turn limit reached").
2. Check the working tree is clean. If not, set `state:human-required` ("working tree not clean before Dev turn").
3. `git checkout main && git pull --ff-only`.
4. Set `state:dev-working`.
5. Build the prompt: the embedded Dev preamble, plus the latest PM prompt. The latest prompt is the newest Huddle comment containing a `<!-- huddle:prompt -->` block; if there is none yet, use the `## First Dev task` section of the control issue body.
6. Run Codex with a 30-minute timeout. Stream its output to the terminal and save the full output to `~/.huddle/logs/<timestamp>-turn<N>-dev.log`.
7. Post a comment on the control issue: turn number, exit code, whether it timed out, the branch now checked out, and the last 50 lines of output in a collapsed `<details>` block. Include the marker `<!-- huddle turn=N role=dev -->`.
8. Set `state:pm-ready`. This happens whatever the Dev outcome: crashes and timeouts go to the PM too.

### PM turn

1. Set `state:pm-working`.
2. Collect facts:
   - the Dev turn's exit code, timeout flag and output tail (from the previous comment or kept in memory),
   - the current branch and `git log main..HEAD --oneline`,
   - the PR for that branch, if any (`gh pr list --head <branch> --json number,url,state`), and its checks (`gh pr checks <n>`),
   - open and closed issue counts in the control issue's milestone.
3. Build the prompt: the embedded PM preamble, the facts, and the verdict instructions (section 9).
4. Run Claude Code with a 30-minute timeout; log to `~/.huddle/logs/<timestamp>-turn<N>-pm.log`.
5. Parse the verdict (section 9). If invalid, re-run the PM once with: "Your last reply did not contain a valid verdict. Reply with only the JSON verdict." If still invalid, set `state:human-required` ("PM verdict unreadable").
6. Act on the verdict:
   - `continue`: post a comment with the reason and the next prompt inside a `<!-- huddle:prompt -->` block (with the target issue), marker `<!-- huddle turn=N role=pm -->`, then set `state:dev-ready`.
   - `done`: check the milestone has no open issues other than the control issue. If it does, set `state:human-required` ("PM said done but issues are open: #x, #y"). Otherwise post the reason, set `state:done`, and close the control issue.
   - `human`: post the reason, set `state:human-required`.

### Limits

- **Turn limit: 20** per control issue. Count = the number of Huddle comments with a `<!-- huddle turn=` marker on the control issue. No local state file is needed.
- **Timeout: 30 minutes** per agent run. On timeout, kill the whole process group.
- **Ctrl+C:** kill the running agent's process group, set `state:human-required` ("stopped by user"), exit.
- **`--once`:** run a single turn, then exit. Used for the pilot.

## 9. PM verdict

The PM must end its reply with a JSON object:

```json
{
  "verdict": "continue",
  "target_issue": 60,
  "next_prompt": "Implement ...",
  "reason": "PR #71 meets the acceptance criteria for #59."
}
```

- `verdict` is one of `continue`, `done`, `human`.
- `reason` is always required.
- `target_issue` (integer) and `next_prompt` (non-empty) are required for `continue`.

Parsing, in order:

1. Extract the answer text from Claude Code's JSON envelope.
2. Find the **last** JSON object in the text (it may be inside a ```` ```json ```` fence or surrounded by prose) and validate it.
3. Fallback: if there is no valid JSON, match the last non-empty line against `continue`, `done`, `human`, ignoring case, markdown and punctuation. `done` and `human` are accepted this way with reason "(no reason given)". A bare `continue` is invalid, because it has no prompt.
4. Otherwise: invalid (triggers the single repair retry).

Write table-driven tests covering messy real-world outputs: fenced JSON, prose after the JSON, two JSON objects, trailing commas, wrong case, missing fields.

## 10. Prompt preambles

Embed both with `go:embed` from `prompts/dev.md` and `prompts/pm.md`.

**Dev preamble** (outline):
- You are the Dev agent on `sohaibseniorswe-sys/palantir`. Read `AGENTS.md` and `Session.md` first.
- Your task is below. Work on branch `agent/issue-<target>` (create it, or check it out if it exists and continue the work there).
- Commit, run the tests, push, and open or update a PR that references the issue.
- Before finishing, update `Session.md` with what you did, what is left, and any questions.

**PM preamble** (outline):
- You are the PM for milestone "<milestone>". Read `AGENTS.md` and `Session.md` first.
- Here are the facts from the last Dev turn. You may use the allowed read-only `gh` commands to inspect the PR, diff and checks.
- Decide: should the Dev continue (same issue with fixes, or the next issue), is the milestone done, or does the human need to step in? Escalate to the human when requirements are unclear, the same problem has failed twice, or a decision is needed that is not in the issue.
- Update `Session.md` with your decision and reasoning. Do not edit any other file.
- End your reply with the JSON verdict (exact schema included in the prompt).

## 11. Suggested code layout

```
huddle/
├── cmd/huddle/main.go        flags, startup, signal handling
├── internal/
│   ├── checks/               startup checks
│   ├── gh/                   gh wrapper behind an interface (fakeable in tests)
│   ├── gitx/                 git helpers
│   ├── runner/               run an agent: timeout, process group, env stripping, logging
│   ├── verdict/              envelope extraction, parsing, validation
│   ├── loop/                 state machine and turns
│   └── prompts/              go:embed templates
├── prompts/dev.md
├── prompts/pm.md
├── README.md
└── MVP_PLAN.md               this file
```

Shell out to `git`, `gh`, `codex` and `claude`. Prefer the standard library; avoid dependencies unless they clearly earn their place.

---

## Milestones

Estimated total: **10 to 16 hours**, most of the uncertainty in Milestone 5.

### Milestone 1: Skeleton and startup checks (2–3 h)

**1.1 Project skeleton**
- Go module, layout from section 11, `huddle --version`, `--yes`, `--once` flags (no-ops for now).
- Acceptance: `go build ./...` and `go test ./...` pass; `huddle --version` prints a version.

**1.2 Local repo checks**
- Section 7 checks 1, 2 and 8. Remote URL parsing for HTTPS and SSH forms.
- Acceptance: unit tests for URL parsing (at least 6 forms) and for the clean-tree check ignoring `Session.md`.

**1.3 GitHub checks**
- `gh` wrapper interface plus a fake for tests. Section 7 checks 3, 4 and 7. Idempotent label creation.
- Acceptance: tests with the fake cover zero, one and several control issues, and a control issue without a milestone.

**1.4 Billing guard and confirmation**
- Section 7 checks 5, 6 and 9. Investigate each CLI's `--help` for a login-status command and report findings.
- Acceptance: Huddle refuses to start when either API key variable is set (tested). Running `huddle` in a real `palantir` clone prints the startup summary and asks to continue.

### Milestone 2: Agent runner (2–3 h)

**2.1 Runner**
- Run a command in a directory with a timeout; stream to terminal and log file; capture the last N lines; kill the whole process group on timeout or cancel; strip both API key variables from the child environment.
- Acceptance: tests using a fake agent script cover normal exit, non-zero exit, timeout (process group killed), and env stripping (child cannot see the variables).

**2.2 Hardcoded agent commands**
- Dev and PM command builders from section 6. Verify every flag against the installed CLIs; update the commands if needed; record versions and final commands in `README.md`.
- Acceptance: a manual smoke test runs each CLI once with a trivial prompt ("Reply with the word OK") from the `palantir` clone, and the README documents the result.

### Milestone 3: Control issue and facts (1–2 h)

**3.1 Control issue operations**
- Set state (removing other `state:*` labels), post comments with markers, find the latest `huddle:prompt` block, read the first Dev task from the issue body, count turns from markers.
- Acceptance: tests with the fake `gh` for each operation, including "no prompt comment yet" and malformed issue bodies.

**3.2 Facts collector**
- Branch, commits since `main`, PR for branch, PR checks, milestone open and closed issues.
- Acceptance: tests with fakes, including "no PR yet" and "checks pending".

### Milestone 4: Verdict and turn loop (2–3 h)

**4.1 Verdict parser**
- Section 9, including envelope extraction and the table-driven tests listed there.
- Acceptance: all listed messy-output cases have tests and pass.

**4.2 Prompts**
- Embedded preambles from section 10, rendered with milestone name, target issue, facts and verdict schema.
- Acceptance: golden-file tests for rendered Dev and PM prompts.

**4.3 State machine and loop**
- Section 8 in full: Dev turn, PM turn, repair retry, `done` verification, interrupted-run detection, human-required polling, turn limit, Ctrl+C, `--once`.
- Acceptance: tests with a fake runner and fake `gh` cover: dev → pm → continue → dev; pm `done` with open issues → human-required; invalid verdict twice → human-required; turn limit; interrupted run at startup; dirty tree before a Dev turn.

### Milestone 5: Pilot on palantir #58 (3–5 h)

**5.1 Single turns**
- With `--once`, run one Dev turn, inspect the comment, PR and `Session.md`; then one PM turn, inspect the verdict comment. Fix what breaks.

**5.2 Full pilot success test**
- Run unattended until the loop reaches `human-required` or `done`. The pilot passes when this sequence happens without manual copy-pasting and without API charges:
  1. PM assigns a small task.
  2. Codex creates a PR.
  3. CI runs.
  4. PM requests a correction.
  5. Codex fixes it.
  6. PM accepts.
  7. Huddle records completion.
- Acceptance: the sequence above is visible in the control issue's comment history; findings and fixes are written to `docs/PILOT.md`.

**5.3 README**
- Install, prerequisites (section 4), usage, flags, what each label means, troubleshooting for everything hit during the pilot.

---

## Out of scope for the MVP

Config files, a SQLite checkpoint, a lock file, background service, push notifications, a reviewer agent, multiple control issues, automatic merging, a separate GitHub account for agents. These are in the full design and come after the pilot passes.
