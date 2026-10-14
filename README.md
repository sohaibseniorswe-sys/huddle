# Huddle

Huddle lets two AI coding agents complete a GitHub milestone from design to delivery in one go: Codex CLI as the Dev agent, Claude Code as the PM agent. The agents talk through a control issue: they post their own comments on it and set its `state:*` label at the end of each turn. Huddle watches the label and starts whichever agent's turn it is; it adds no limits, timeouts or restrictions. The human is involved only if the PM sets `state:human-required`, or Huddle does for an account or credit error. The spec is [MVP_PLAN.md](MVP_PLAN.md).

**Status:** Huddle v3 is in progress ([docs/v3](docs/v3/README.md)). v3.1 and v3.2 are done: after the startup checks, `huddle` shows a full-screen run screen (below) with a companion agent you talk to while the Dev and PM work. `huddle --plain` behaves like v1.

## Build and run

```bash
go build -o huddle ./cmd/huddle
./huddle --version

# Run from inside a clone of the target repo:
cd ~/projects/huddle-sandbox && huddle --repo sohaibseniorswe-sys/huddle-sandbox
```

| Flag | Meaning |
|------|---------|
| `--repo owner/name` | Target repo. Default `sohaibseniorswe-sys/palantir`. Use the sandbox for testing. |
| `--yes` | Skip the `Continue? [Y/n]` prompt. |
| `--once` | Run a single agent turn (including its resume fallback, usage-limit wait or escalation, and anti-stall), then stop the loop. With `--plain` Huddle then exits; on the run screen it stays open until you quit. |
| `--plain` | No run screen: print Huddle's lines and the agents' raw output to stdout, exactly as v1 did. For tests, CI and piping into a file. |
| `--version` | Print the version. |

## The run screen

After the startup checks, Huddle takes over the terminal with a fixed layout (`internal/app`), from top to bottom:

- **Status bar and info line:** repo, milestone and control issue; a badge saying what is happening (`Dev (Codex) working`, `PM (Claude) working`, `Waiting for you`, `Paused · PM next`, `Waiting for usage reset · 03:45`, `Done`); issues closed, open PRs, CI on `main`, usage per agent and elapsed time. The GitHub facts are refreshed every minute and after every turn.
- **Activity:** one line per step, newest at the bottom. Huddle's own steps, plus events read from the agents' output while they run: each Codex message's first sentence, each shell command as `$ <command>   ok` (or its exit code), and the first sentence of Claude Code's result.
- **Companion** (when open): your conversation with the companion; see below.
- **Slot** (when needed): the `/` menu, or a decision or action card waiting for your answer.
- **Text field** and a **help line** with the keys that work right now.

The terminal must be at least 80 × 24.

**Slash commands.** Type `/` to open the command menu; typing filters it, ↑/↓ choose, Tab completes, Enter runs, Esc closes. Only commands that make sense right now are shown.

| Command | Does |
|---|---|
| `/status` | Facts in the Activity pane: turn, issues, open PRs and their CI, `main`, usage, elapsed. Starts no agent. |
| `/log` | The agents' raw logs for this repo, at the end of the newest run. `[` `]` older/newer run, `g` `G` start/end, PgUp/PgDn scroll. |
| `/pause` | The running turn finishes normally; no new turn starts. |
| `/resume` | Start the next turn now. |
| `/companion` | Opens or closes the Companion pane (● while open). Closing only hides it; the conversation is kept. |
| `/decide` | After you have talked with the companion: asks it to draft the decision you agreed, as a card. |
| `/decision` | While a card waits: moves the keyboard to the card. |
| `/quit` | Same as Ctrl+C: stops the running agent (and a companion still thinking), puts its ready label back, exits. |

**Focus.** Keys go only to the focused element: the text field (default), the menu, the Activity pane, the Companion pane or a card (purple border, "◆ focused"). Tab cycles text field → Activity → Companion. In Activity, ↑/↓ select a line, Enter opens the raw log at that line, Esc goes back. Typing `/` in a pane opens the menu there too, and whatever you had half-typed in the text field comes back when the menu closes. The mouse works too: the wheel scrolls the pane under the pointer, and a click focuses a pane, selects an Activity line or presses a card's button. While Huddle has the mouse, the terminal's own text selection needs Shift + drag in most terminals.

### The companion

The human never talks to the Dev or the PM. You talk to the **companion**, a third agent that knows everything about the run and never changes anything (`internal/companion`).

- **Talking.** Plain text in the text field (prefix `companion ›`) goes to the companion; the Companion pane opens and shows "companion is thinking…" while it answers. Each message is one print-mode run of Claude Code (the PM's CLI, default model; v3.3 makes this configurable) in its own session, resumed by later messages. The Dev and PM keep working meanwhile: the companion runs beside the loop, and neither waits for the other. Messages you send while it is still answering go together in its next run. Nothing you type is posted anywhere.
- **What it knows.** Every message is wrapped in a short `HUDDLE CONTEXT` block that Huddle builds fresh: what your screen shows (for example the raw log of Codex's turn 5, opened at 22:52:07 from a given Activity line), the state label and turn, the milestone's closed and open issues, the last 15 Activity events, the newest 3 control issue comments, the PM's last output, the running agent's last 40 output lines and any card waiting. The first message of a session also carries the companion's instructions (`internal/companion/instructions.md`). It can also read the repo, issues and PRs and search the web.
- **When the PM needs you.** When the control issue gets `state:human-required`, Huddle opens the Companion pane and asks the companion to explain the PM's question (its latest comment) before you type anything.
- **Decisions.** When you agree on something the PM must act on (or use `/decide`), the companion writes it in `<decision>` and `<reason>` tags. Huddle shows it as a **decision card**: the statement, `Agree` and `Keep talking`. The text field keeps the keyboard while a card waits (its buttons are greyed out); `/decision` or a click moves the keyboard to the card, where ←/→ choose, Enter confirms and Esc goes back to the text field. Only one card waits at a time; a newer one replaces it.
- **Agree** posts the decision on the control issue as you (your `gh` login), marked `<!-- huddle:decision -->`, with its reason. If the control issue says `state:human-required`, Huddle also sets `state:pm-ready` and the PM starts at once; otherwise the label stays and the PM reads the decision at its next turn (its instructions say to act on `huddle:decision` comments first). **Keep talking** posts nothing. If posting fails, the card comes back.
- **Actions.** The companion may propose a Huddle action (`pause`, `resume`, `status`, `log HH:MM`) as an **action card** with `Agree` and `Not now`; it runs only if you agree.
- **Files.** The conversation is saved to `~/.huddle/companion/<owner>_<repo>_<control-issue>.jsonl` and comes back when Huddle restarts. The companion's session id is in `~/.huddle/sessions/<owner>_<repo>_<control-issue>_companion.json`, next to the Dev and PM file (a file of its own, because the companion saves while the loop may be saving). Its raw output goes to `~/.huddle/logs/companion/`, apart from the runs `/log` shows.

## Startup checks

Huddle runs these checks in order and stops at the first failure, printing the reason and exiting with status 1 (section 7 of the plan):

1. Neither `OPENAI_API_KEY` nor `ANTHROPIC_API_KEY` is set.
2. The current directory is in a git repo whose `origin` is the target repo on github.com.
3. `AGENTS.md` and `CLAUDE.md` exist. `Session.md` is created empty if it is missing.
4. `gh` is logged in and can see the target repo.
5. All Huddle labels exist on the target repo (missing ones are created; existing ones are left alone).
6. `codex` and `claude` are on `PATH` and logged in with a subscription.
7. Exactly one open issue is labelled `control`, and it has a milestone.
8. Huddle prints the summary and asks `Continue? [Y/n]` (skipped with `--yes`).

There is no clean-tree check: after an interrupted run the tree is normally dirty, and Huddle must be able to resume the agent.

## The watch loop

Once the checks pass, Huddle watches the control issue (`internal/loop`). It reads the issue's `state:*` label every 30 seconds while no agent is running (at once after `/resume` or an agreed decision), and straight after each agent exits. Only one Dev or PM agent runs at a time; the companion may run beside it. Huddle never stops on its own: the loop ends on `state:done` (status 0), on Ctrl+C or `/quit` (status 130), or after one turn with `--once`. On the run screen, Huddle then stays open until you quit; with `--plain` it exits.

To start a run, add `state:dev-ready` to the control issue (or `state:pm-ready` to have the PM write the first Dev prompt).

### Labels

| Label | Set by | What Huddle does |
|-------|--------|------------------|
| `control` | human | Marks the control issue. |
| `state:dev-ready` | agents, or the human to start | Runs a Dev (Codex) turn. |
| `state:dev-working` | Huddle, while Codex runs | Found at startup, it means a previous Huddle was interrupted: Codex is started again, resuming its session. |
| `state:pm-ready` | agents, or the human | Runs a PM (Claude Code) turn. |
| `state:pm-working` | Huddle, while the PM runs | As `dev-working`, for the PM. |
| `state:human-required` | PM, or Huddle for account and credit errors | Prints the latest control-issue comment (once per new comment) and keeps polling until someone sets a ready label. |
| `state:done` | PM | Prints the PM's final report (the latest comment) and exits 0. |
| none | | Prints "add state:dev-ready to start" once and keeps polling. |

Agents set labels with `gh issue edit --add-label` and may leave the old one in place, so Huddle normalises: if several state labels are present, the first of `human-required`, `done`, `pm-ready`, `dev-ready`, `pm-working`, `dev-working` wins and the others are removed. Whenever Huddle sets a label it removes every other `state:*` label.

### A turn

1. Huddle sets `state:dev-working` or `state:pm-working`.
2. It sends the launch prompt (the operating principles, below) on stdin and runs the agent, resuming its saved session if there is one. Output is saved to `~/.huddle/logs/<timestamp>-turn<N>-<role>.log`, each line starting with the time it arrived (`22:52:07  `), and turned into Activity events while the agent runs; with `--plain` it is also streamed to the terminal. There is no timeout.
3. When the agent exits, Huddle saves its session id and reads the label again. If the agent set a new label, that decides what happens next.
4. If the label is unchanged, Huddle checks the output for usage-limit messages first (below), then applies the resume fallback, then anti-stall.

**Anti-stall.** If an agent exits without setting a new `state:*` label, whatever its exit code, Huddle posts a comment on the control issue with the agent's final message and exit code, marked `<!-- huddle:auto-handoff role=dev|pm -->`, and hands the turn to the other agent: `state:pm-ready` after Codex, `state:dev-ready` after the PM.

**Usage limits.** A limited run never hands over and never forgets its session. Huddle posts one comment marked `<!-- huddle:usage-wait role=dev|pm -->` (with the reset time if the CLI gave one), keeps the `*-working` label, sleeps until the reset plus one minute (or retries every 15 minutes if there is no usable reset time), then resumes the same session with the normal launch prompt. Retries within the same wait post no new comment. An account or credit error (see Usage-limit messages) instead gets one comment marked `<!-- huddle:escalate role=dev|pm -->` with the CLI's exact message, and `state:human-required`. The agent's session is kept, so it resumes when the human sets its ready label again. This is the only time Huddle itself sets `state:human-required`.

**GitHub errors.** If a `gh` call fails, Huddle prints a warning and retries it every 30 seconds rather than stopping.

### Sessions

Huddle keeps one session id per agent per control issue in `~/.huddle/sessions/<owner>_<repo>_<control-issue>.json` (`{"dev": "...", "pm": "..."}`), so sessions survive a restart of Huddle. The companion's is in `<owner>_<repo>_<control-issue>_companion.json` next to it. Each run's session id replaces the saved one. **Fallback:** if a resumed run exits non-zero without setting a new label (and without a usage-limit message), Huddle forgets that session and runs the same turn once more in a fresh session; only if that run also leaves the label unchanged does anti-stall apply. To start an agent afresh, delete the session file or its entry. Each control issue has its own file, so a new control issue starts with fresh sessions.

### Ctrl+C

Ctrl+C (or `/quit` on the run screen) reaches only Huddle (each agent runs in its own process group). If an agent is running, Huddle kills its whole process group, saves its session id, puts back that agent's ready label so the next `huddle` resumes it, and exits 130. The same applies while Huddle is waiting out a usage limit. While no agent is running, Huddle just exits 130. If Huddle is killed outright (no Ctrl+C), the `*-working` label stays, and the next `huddle` treats it as that agent's ready label.

## Decisions

- **The API key check runs first.** The plan originally listed it fifth. Running it first means Huddle does nothing at all (no git, `gh` or agent CLI calls) while an API key is in the environment. A key that is set to an empty value also counts as set.
- **Strict subscription login checks for both CLIs.** Huddle requires the exact subscription login reported by each CLI's status command (below) and rejects anything else, including login types it has not seen. If a legitimate subscription login is rejected, the error message shows the value found, and the check can be widened in `internal/checks/agents.go`.
- **github.com-only remotes.** The `origin` host must be `github.com` (or `ssh.github.com` for SSH over port 443). SSH host aliases such as `git@github-work:owner/repo` are rejected; use a plain github.com remote URL in the target clone.

## CLI login-status findings

Both CLIs have a free, local login-status command, so Huddle uses them. These were checked on 2026-10-12 with the versions below.

| CLI | Version | Command | Subscription output | Huddle requires |
|-----|---------|---------|---------------------|-----------------|
| Codex | `codex-cli 0.160.0` | `codex login status` | `Logged in using ChatGPT` (printed on **stderr**, exit 0) | Output contains `Logged in using ChatGPT` |
| Claude Code | `2.1.288` (rechecked on `2.1.296`) | `claude auth status --json` | JSON with `"loggedIn": true`, `"authMethod": "claude.ai"`, `"apiProvider": "firstParty"`, `"subscriptionType": "pro"` | `loggedIn` true, `authMethod` = `claude.ai`, `apiProvider` = `firstParty` |

Notes:

- `codex login --with-api-key` exists. Its status text was not observed, but it does not say "ChatGPT", so it is rejected.
- `claude auth status` defaults to JSON; `--text` prints e.g. `Login method: Claude Pro account`. Huddle parses the JSON.
- Requiring `apiProvider` = `firstParty` also rejects Bedrock and Vertex setups, which bill through a cloud account.
- A `claude setup-token` login has not been observed. If its `authMethod` is not `claude.ai`, Huddle rejects it (see Decisions).

## Agent runner

`internal/runner` runs one agent at a time: in the repo root, with no time limit, with output saved to `~/.huddle/logs/<timestamp>-turn<N>-<role>.log` (each line time-stamped, and passed to `Spec.OnLine` for the Activity pane) and streamed to the terminal only with `--plain`, and with the first 30 and last 50 lines kept (`Result.Head`, `Result.Tail`). Each agent runs in its own process group, and the whole group gets `SIGKILL` when the run ends: on Ctrl+C, and after a normal exit too, so nothing the agent started in the background outlives its turn. If a leftover process still holds the output pipes, the run waits up to 5 s for them before closing them. `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` are removed from the child environment even though startup already refuses to run with them set. A non-zero exit or a cancel is reported in the result, not returned as an error. `Spec.Timeout` still exists for tests; zero (the default, and all Huddle uses) means no timeout.

## Agent commands

Checked on 2026-10-12 against `codex exec --help`, `codex exec resume --help` and `claude --help`, and by running each command (fresh and resumed) in the sandbox. Both agents run with full permissions and no allow or deny lists; both read the prompt from stdin.

| Agent | Version | Fresh | Resume |
|-------|---------|-------|--------|
| Dev | `codex-cli 0.160.0` | `codex exec --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message <file> -` | `codex exec resume --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message <file> <session-id> -` |
| PM | Claude Code `2.1.296` | `claude -p --output-format json --dangerously-skip-permissions` | `claude -p --output-format json --dangerously-skip-permissions --resume <session-id>` |
| Companion | Claude Code `2.1.299` | `claude -p --output-format json --tools Read,Grep,Glob,WebSearch,WebFetch,Bash --permission-mode dontAsk --strict-mcp-config --allowedTools <rules>` | the same with `--resume <session-id>` before `--allowedTools` |

### Dev (Codex)

- **Full access.** `-c sandbox_mode="danger-full-access"` and `-c approval_policy="never"`; the header of every run, fresh or resumed, confirms `approval: never` and `sandbox: danger-full-access`. These are `-c` overrides rather than `--sandbox` because `codex exec resume` has no `--sandbox` flag but does accept `-c`. `--dangerously-bypass-approvals-and-sandbox` would also work but was not needed.
- **`--ignore-user-config`** skips `~/.codex/config.toml`, so a user's config cannot change the run. Auth still comes from `CODEX_HOME`. **`--strict-config`** makes codex reject a misspelt `-c` key or value instead of ignoring it.
- **`codex exec resume`** accepts `--strict-config`, `--ignore-user-config`, `-c` and `--output-last-message`, placed after `resume` and before the session id. With `-` as the prompt it reads stdin (checked: it answered from the earlier turn's context). A resumed run keeps the same session id. A bad id fails fast: `Error: thread/resume: thread/resume failed: no rollout found for thread id <id> (code -32600)`, exit 1.
- **Session id.** `codex exec` prints a header on stderr before it starts work, with a line `session id: <uuid>`, for fresh and resumed runs alike. Huddle reads it from the first lines of output (`runner.CodexSessionID`). `--json` would give the same id as the `thread_id` of the first `thread.started` event, but it turns the terminal stream into JSONL, so Huddle uses the header. `--last` is never used.
- **Final message.** `--output-last-message <file>` ("Specifies file where the last message from the agent should be written"). Huddle reads that file (`runner.CodexFinalMessage`). If it is empty or missing it falls back to stdout, where `codex exec` prints only the final message, then to the output tail.
- **Removed:** `workspace-write`, `sandbox_workspace_write.network_access` and the `--add-dir` Go cache directories, which the restricted sandbox needed.

### PM (Claude Code)

- **`--dangerously-skip-permissions`** ("Bypass all permission checks") exists in 2.1.296 under that name. No `--permission-mode`, no allow or deny lists of tools, no `--add-dir`.
- **`--resume <session-id>`** resumes by id; `--continue` (most recent conversation in the directory) is never used. A bad id prints `No conversation found with session ID: <id>` and exits 1.
- **JSON envelope.** With `--output-format json`, stdout is a single JSON object (`runner.ParseClaude`). `session_id` is the session id (the same on resume); `result` is the final message; `is_error` and `subtype` report failures. `total_cost_usd` is a notional list-price figure (`"costBasis": "list"`); runs use the claude.ai subscription login with no API key in the environment, so nothing is billed.

### Companion (read-only)

Checked on 2026-10-14 against `claude --help` (Claude Code `2.1.299`) and `codex exec --help` / `codex exec resume --help` (`codex-cli 0.160.0`), and by running each in the sandbox.

- **Claude Code** (used in v3.2). `--tools Read,Grep,Glob,WebSearch,WebFetch,Bash` ("Specify the list of available tools from the built-in set") leaves out every tool that edits or writes files. `--permission-mode dontAsk` denies every tool call that no allow rule permits, and `--allowedTools` allows only the reading tools plus read-only shell commands: `git log/show/diff/status/blame/ls-files/grep/rev-parse` and `gh issue view/list`, `gh pr view/list/checks/diff`, `gh run list/view` (`runner.CompanionAllowed`). `--strict-mcp-config` without `--mcp-config` loads no MCP servers. `--allowedTools` takes every argument after it, so it goes last. `--permission-mode plan` was not used: in print mode it is less predictable than an explicit allow list.
  - Checked: asked to create, commit and push a file and to comment on an issue, the bare command (without the companion's instructions) was denied every Bash call and had no file tool; nothing changed. `git log` and `gh issue view` worked. `TestSmokeCompanionIsReadOnly` repeats this (`HUDDLE_SMOKE=~/projects/huddle-sandbox go test -count=1 -run Smoke -v ./internal/companion`).
  - Limits: allow rules from your own Claude Code settings still apply, so a broad rule there (for example `Bash(git commit:*)`) would let the companion run it; the companion's instructions also forbid any change. `git diff --output=<file>` can write a file.
- **Codex** (for v3.3, not used yet). `codex exec --strict-config --ignore-user-config -c sandbox_mode="read-only" -c approval_policy="never"`; `-c` rather than `--sandbox read-only` because `codex exec resume` has no `--sandbox`. The run header confirms `sandbox: read-only`; asked to create and commit a file, the shell failed with `Read-only file system` and nothing changed. The read-only sandbox also blocks the network, so `gh` calls fail: a Codex companion relies on the context block.

### Smoke test

`TestSmokeRealAgents` runs each real CLI fresh with "Remember the word PELICAN. Reply with only the word OK.", then resumes the same session by its extracted id and asks for the word back. It is skipped unless `HUDDLE_SMOKE` names a target clone:

```bash
HUDDLE_SMOKE=~/projects/huddle-sandbox go test -count=1 -run Smoke -v ./internal/runner
```

Result on 2026-10-12 in `~/projects/huddle-sandbox`: both passed. Codex: 8 s fresh, 7 s resumed, same session id, answered `OK` then `PELICAN`. Claude: 3 s fresh, 4 s resumed, same `session_id`, `result` `OK` then `PELICAN`.

The unit tests use recorded real output in `internal/runner/testdata/`.

## Usage-limit messages

Taken from the installed CLIs on 2026-10-12. Codex is open source and its strings were read from the installed binary; Claude Code's were read from its installed bundle, including the function that builds the message for each limit type and Claude Code's own list of limit-message prefixes. Neither limit has been hit in practice yet, so the exact output as printed by `codex exec` and `claude -p` is still unobserved. Huddle matches only the messages below, at the start of a line (optionally after an `ERROR:`, `Error:` or `API Error:` prefix), so test output that merely mentions a limit is not mistaken for one (`runner.CodexUsageLimit`, `runner.ClaudeUsageLimit`). Each message below has a test case in `internal/runner/usage_test.go`.

Each message is handled in one of two ways (MVP_PLAN.md section 8.6):

- **Wait.** Any message with a reset time: Huddle sleeps until the reset plus one minute, then resumes the same agent. A plain usage limit (session, weekly, model-specific and similar plan limits) with no reset time, or with one in a format Huddle can't parse, also waits, retrying every 15 minutes.
- **Escalate.** Account-setup errors, and spend-cap, out-of-credits and spend-limit messages with no reset time. Only money or account changes can fix these, so Huddle posts the CLI's exact message (without the `ERROR:` prefix) as the reason, with the marker `<!-- huddle:escalate role=dev|pm -->`, and sets `state:human-required`. The agent's session is kept for when the human sets a ready label again.

If one run's output contains several of these, a message with a reset time wins, then an escalation, then a plain usage limit.

### Codex (`codex-cli 0.160.0`)

Codex uses a typographic apostrophe (U+2019) in most of these; a plain one is accepted too. The core messages are reassembled from string fragments in the binary, so the joins between fragments are inferred.

**Usage limit: wait** (`UsageLimitReachedError`, printed by `codex exec` as an error line). The ending is ` Try again at <time>.`, `, or try again at <time>.` / ` or try again at <time>.`, or ` Try again later.` when the reset time is unknown.

| Variant | Message |
|---------|---------|
| Plain | `You’ve hit your usage limit. Try again at <time>.` |
| Plus plan | `You’ve hit your usage limit. Upgrade to Plus to continue using Codex (https://chatgpt.com/explore/plus), or try again at <time>.` |
| Pro upgrade | `You’ve hit your usage limit. Upgrade to Pro (https://chatgpt.com/explore/pro), visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at <time>.` |
| Pro / credits | `You’ve hit your usage limit. Visit https://chatgpt.com/codex/settings/usage to purchase more credits or try again at <time>.` |
| Team / Business | `You’ve hit your usage limit. To get more access now, send a request to your admin or try again at <time>.` |
| Model-specific limit | `You’ve hit your usage limit for <model>. Switch to another model now, or try again at <time>.` |

`<time>` is in the machine's **local time zone**: `3:45 PM` if the reset is today, otherwise `Oct 13th, 2026 3:45 PM` (chrono formats `%-I:%M %p` and `%b %-d<st|nd|rd|th>, %Y %-I:%M %p`).

**Workspace spend control: escalate** (no reset time):

- `Your workspace is out of credits. Add credits to continue.`
- `Your workspace is out of credits. Ask your workspace owner to refill in order to continue.`
- `You hit your spend cap set in your workspace. Increase your spend cap to continue.`
- `You hit your spend cap set by the owner of your workspace. Ask an owner to increase your spend cap to continue.`

**Plan does not include Codex: escalate** (`UsageNotIncluded`):

- `To use Codex with your ChatGPT plan, upgrade to Plus: https://chatgpt.com/explore/plus.`

**API quota: escalate:**

- `Quota exceeded. Check your plan and billing details.` This only appears with API-key billing, which should never happen: Huddle refuses to start with an API key set and strips both keys from the agents' environment. If it appears anyway, the Codex login is not the ChatGPT subscription it should be, so Huddle stops and asks the human.

**Interactive (TUI) notices**, not expected from `codex exec` but matched in case they appear (no reset time):

- `Usage limit reached. You’ve reached your usage limit. Increase your limits to continue using codex.` (wait, 15-minute retries)
- `You’ve reached your workspace credit limit` (escalate)
- `You’re out of credits. Your workspace is out of credits. Add credits to continue using Codex.` (escalate)

**Not matched:** `exceeded retry limit, last status: 429 ...` (transient rate limiting that codex retries itself).

### Claude Code (`2.1.296`)

In `-p --output-format json` mode the message is the envelope's `result` (with `is_error: true`); Huddle also checks the raw output. Messages are built as `You've hit your <limit>` followed by ` · `-separated parts: advice, the reset, and optionally ` · progress saved`.

**Plan limits: wait**, with or without a reset time (`You've hit your <limit> · resets <time>[ · progress saved]`):

| Limit type | `<limit>` |
|------------|-----------|
| 5-hour (`five_hour`) | `session limit` |
| Weekly (`seven_day`) | `weekly limit` |
| Weekly Opus (`seven_day_opus`) | `Opus limit` |
| Weekly Sonnet (`seven_day_sonnet`) | `Sonnet limit` (`weekly limit` on Pro and Enterprise plans) |
| Weekly Fable (`seven_day_overage_included`) | `Fable limit` |
| Other / unknown type | `usage limit`, or `limit` when usage credits are also exhausted |

For example `You've hit your weekly limit · resets Oct 15, 10am (Europe/London)` or `You've hit your Opus limit · resets 9:30am (Europe/London)`. With no reset time, `You've hit your usage limit · contact your admin to increase it` (15-minute retries). `Usage limit reached · limit resets <time>` (the wait notice) and the older format `Claude AI usage limit reached|<unix seconds>` are plan limits too.

**Spend and credit limits: wait if they give a reset time, otherwise escalate.** The reset, if any, is given as ` · your <plan limit> resets <time>` or ` · resets <time>`:

- `You've hit your monthly spend limit · raise it at <url>[ · your <limit> resets <time>]` (also `You've hit your monthly spend limit.`)
- `You've hit your org's monthly spend limit · ...`, `You've hit your individual spend limit · ...`
- `You've hit your individual usage limit[ · resets <time>]`, `You've hit your org's monthly usage limit[ · resets <time>]`
- `You've hit your channel's monthly spend limit · an org owner or channel manager can raise it in the channel's Claude settings[ · ...]`, and the same with `channel's monthly usage limit`
- `You've hit your team's shared budget · raise it at <url>[ · your <limit> resets <time>]`
- `You've reached your Fable limit. Switch to another model to continue.`
- `You're out of usage credits[ · resets <time>][ · progress saved]`, `You're out of extra usage`, `Your org is out of usage · add funds to continue` / `· contact your admin`

**Account-setup errors: always escalate**, since no wait will fix them:

- `Your seat type doesn't include usage credits`, `Your seat type doesn't include usage`, `Your seat type doesn't include extra usage`
- `This service is disabled for your org`
- `Your usage allocation has been disabled by your admin · ...`
- `Your group's usage limit is set to $0 · ...`
- `<model> requires usage credits. ...`, for example `Opus 5 requires usage credits. Switch to another model to continue.` or `Fable 5 requires usage credits. ...`

`<time>` is `3pm` or `3:30pm` if the reset is within 24 hours (so a time already past today means tomorrow), otherwise `Oct 13, 3pm`, with the year added (`Jan 2, 2027, 3pm`) if it differs; the zone in parentheses is the user's IANA time zone.

**Not matched:** `You've hit your fast limit · resets in <duration>` (a fast-mode cooldown; `-p` runs don't use fast mode) and the warnings `You've used <n>% of your <limit> · ...` and `You're close to your ...`.

## Sessions and the control issue

- `internal/session` stores `{"dev": "...", "pm": "..."}` in `~/.huddle/sessions/<owner>_<repo>_<control-issue>.json`. A missing or unreadable file means no sessions; writes go through a temporary file and a rename.
- `internal/gh` reads and sets the control issue's `state:*` label, removes a label, reads the latest comment and posts comments (bodies go to `gh issue comment --body-file -` on stdin). Reading the state normalises it: if several state labels are present, the first of `human-required`, `done`, `pm-ready`, `dev-ready`, `pm-working`, `dev-working` wins and every other `state:*` label is removed, so a label an agent set always beats a leftover `*-working` label. Setting a state removes every other `state:*` label in the same `gh issue edit`.
- `internal/prompts` embeds `shared.md`, `dev.md` and `pm.md` (the operating principles from MVP_PLAN.md section 9, checked byte for byte by a test) and renders the launch prompt.
