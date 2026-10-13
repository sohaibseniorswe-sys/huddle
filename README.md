# Huddle

Huddle lets two AI coding agents complete a GitHub milestone from design to delivery in one go: Codex CLI as the Dev agent, Claude Code as the PM agent. The agents talk through a control issue: they post their own comments on it and set its `state:*` label at the end of each turn. Huddle watches the label and starts whichever agent's turn it is; it adds no limits, timeouts or restrictions. The human is involved only if the PM sets `state:human-required`. The spec is [MVP_PLAN.md](MVP_PLAN.md).

**Status:** Milestone 3 is done: agent commands with full permissions and session resume, the session store, control-issue operations, usage-limit detection and the embedded operating principles. The watch loop that ties them together comes in Milestone 4; until then `huddle` runs the startup checks and exits. Milestone 1's startup checks still include the clean-tree check, which Milestone 4 removes. The full README (install, usage, labels, troubleshooting) comes in Milestone 5.4.

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
| `--once` | Run a single agent turn, then exit (no-op until the watch loop exists, Milestone 4). |
| `--version` | Print the version. |

## Startup checks

Huddle runs these checks in order and stops at the first failure, printing the reason and exiting with status 1 (section 7 of the plan):

1. Neither `OPENAI_API_KEY` nor `ANTHROPIC_API_KEY` is set.
2. The current directory is in a git repo whose `origin` is the target repo on github.com.
3. The working tree is clean, ignoring `Session.md`. (Old design: removed in Milestone 4, so Huddle can resume an agent after an interrupted run.)
4. `AGENTS.md` and `CLAUDE.md` exist. `Session.md` is created empty if it is missing.
5. `gh` is logged in and can see the target repo.
6. All Huddle labels exist on the target repo (missing ones are created; existing ones are left alone).
7. `codex` and `claude` are on `PATH` and logged in with a subscription.
8. Exactly one open issue is labelled `control`, and it has a milestone.
9. Huddle prints the summary and asks `Continue? [Y/n]` (skipped with `--yes`).

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

`internal/runner` runs one agent at a time: in the repo root, with no time limit, with output streamed to the terminal and saved to `~/.huddle/logs/<timestamp>-turn<N>-<role>.log`, and with the first 30 and last 50 lines kept (`Result.Head`, `Result.Tail`). Each agent runs in its own process group, and the whole group gets `SIGKILL` when the run ends: on Ctrl+C, and after a normal exit too, so nothing the agent started in the background outlives its turn. If a leftover process still holds the output pipes, the run waits up to 5 s for them before closing them. `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` are removed from the child environment even though startup already refuses to run with them set. A non-zero exit or a cancel is reported in the result, not returned as an error. `Spec.Timeout` still exists for tests; zero (the default, and all Huddle uses) means no timeout.

## Agent commands

Checked on 2026-10-12 against `codex exec --help`, `codex exec resume --help` and `claude --help`, and by running each command (fresh and resumed) in the sandbox. Both agents run with full permissions and no allow or deny lists; both read the prompt from stdin.

| Agent | Version | Fresh | Resume |
|-------|---------|-------|--------|
| Dev | `codex-cli 0.160.0` | `codex exec --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message <file> -` | `codex exec resume --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message <file> <session-id> -` |
| PM | Claude Code `2.1.296` | `claude -p --output-format json --dangerously-skip-permissions` | `claude -p --output-format json --dangerously-skip-permissions --resume <session-id>` |

### Dev (Codex)

- **Full access.** `-c sandbox_mode="danger-full-access"` and `-c approval_policy="never"`; the header of every run, fresh or resumed, confirms `approval: never` and `sandbox: danger-full-access`. These are `-c` overrides rather than `--sandbox` because `codex exec resume` has no `--sandbox` flag but does accept `-c`. `--dangerously-bypass-approvals-and-sandbox` would also work but was not needed.
- **`--ignore-user-config`** skips `~/.codex/config.toml`, so a user's config cannot change the run. Auth still comes from `CODEX_HOME`. **`--strict-config`** makes codex reject a misspelt `-c` key or value instead of ignoring it.
- **`codex exec resume`** accepts `--strict-config`, `--ignore-user-config`, `-c` and `--output-last-message`, placed after `resume` and before the session id. With `-` as the prompt it reads stdin (checked: it answered from the earlier turn's context). A resumed run keeps the same session id. A bad id fails fast: `Error: thread/resume: thread/resume failed: no rollout found for thread id <id> (code -32600)`, exit 1.
- **Session id.** `codex exec` prints a header on stderr before it starts work, with a line `session id: <uuid>`, for fresh and resumed runs alike. Huddle reads it from the first lines of output (`runner.CodexSessionID`). `--json` would give the same id as the `thread_id` of the first `thread.started` event, but it turns the terminal stream into JSONL, so Huddle uses the header. `--last` is never used.
- **Final message.** `--output-last-message <file>` ("Specifies file where the last message from the agent should be written"). Huddle reads that file (`runner.CodexFinalMessage`). If it is empty or missing it falls back to stdout, where `codex exec` prints only the final message, then to the output tail.
- **Removed:** `workspace-write`, `sandbox_workspace_write.network_access` and the `--add-dir` Go cache directories, which the restricted sandbox needed.

### PM (Claude Code)

- **`--dangerously-skip-permissions`** ("Bypass all permission checks") exists in 2.1.296 under that name. No `--permission-mode`, `--allowedTools`, `--disallowedTools` or `--add-dir`.
- **`--resume <session-id>`** resumes by id; `--continue` (most recent conversation in the directory) is never used. A bad id prints `No conversation found with session ID: <id>` and exits 1.
- **JSON envelope.** With `--output-format json`, stdout is a single JSON object (`runner.ParseClaude`). `session_id` is the session id (the same on resume); `result` is the final message; `is_error` and `subtype` report failures. `total_cost_usd` is a notional list-price figure (`"costBasis": "list"`); runs use the claude.ai subscription login with no API key in the environment, so nothing is billed.

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

**Interactive (TUI) notices**, not expected from `codex exec` but matched in case they appear (no reset time):

- `Usage limit reached. You’ve reached your usage limit. Increase your limits to continue using codex.` (wait, 15-minute retries)
- `You’ve reached your workspace credit limit` (escalate)
- `You’re out of credits. Your workspace is out of credits. Add credits to continue using Codex.` (escalate)

**Not matched:** `Quota exceeded. Check your plan and billing details.` (API-key billing, which Huddle never uses) and `exceeded retry limit, last status: 429 ...` (transient rate limiting that codex retries itself).

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
