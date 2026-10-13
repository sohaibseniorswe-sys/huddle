# Huddle

Huddle lets two AI coding agents work through a GitHub milestone in turns: Codex CLI as the Dev agent, Claude Code as the PM agent. The human steps in only when the agents escalate. The spec is [MVP_PLAN.md](MVP_PLAN.md).

**Status:** Milestone 2 (agent runner and hardcoded agent commands). The turn loop is not implemented yet. The full README (install, usage, labels, troubleshooting) comes in Milestone 5.4.

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
| `--once` | Run a single turn, then exit (no-op until the turn loop exists). |
| `--version` | Print the version. |

## Startup checks

Huddle runs these checks in order and stops at the first failure, printing the reason and exiting with status 1 (section 7 of the plan):

1. Neither `OPENAI_API_KEY` nor `ANTHROPIC_API_KEY` is set.
2. The current directory is in a git repo whose `origin` is the target repo on github.com.
3. The working tree is clean, ignoring `Session.md`.
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

`internal/runner` runs one agent at a time: in the repo root, with a timeout (30 minutes by default), with output streamed to the terminal and saved to `~/.huddle/logs/<timestamp>-turn<N>-<role>.log`, and with the last 50 lines kept for the control-issue comment. Each agent runs in its own process group, and the whole group gets `SIGKILL` when the run ends: on timeout, on Ctrl+C, and after a normal exit too, so nothing the agent started in the background outlives its turn. If a leftover process still holds the output pipes, the run waits up to 5 s for them before closing them. `OPENAI_API_KEY` and `ANTHROPIC_API_KEY` are removed from the child environment even though startup already refuses to run with them set. A non-zero exit, a timeout or a cancel is reported in the result, not returned as an error.

## Agent commands

Checked against the installed CLIs on 2026-10-12 with `codex exec --help`, `claude --help` and the Claude Code [permissions docs](https://code.claude.com/docs/en/permissions). Both commands are self-contained: neither depends on the user's CLI config for sandboxing or permissions.

| Agent | Version | Final command (prompt on stdin) |
|-------|---------|---------------------------------|
| Dev | `codex-cli 0.160.0` | `codex exec --strict-config --ignore-user-config -c sandbox_mode="workspace-write" -c approval_policy="never" -c sandbox_workspace_write.network_access=true --add-dir <GOMODCACHE> --add-dir <GOCACHE> -` |
| PM | Claude Code `2.1.296` | `claude -p --output-format json --permission-mode dontAsk --allowedTools Read "Edit(/Session.md)" "Bash(git log *)" "Bash(git diff *)" "Bash(git status *)" "Bash(git show *)" "Bash(gh issue view *)" "Bash(gh issue list *)" "Bash(gh pr view *)" "Bash(gh pr list *)" "Bash(gh pr diff *)" "Bash(gh pr checks *)" --disallowedTools "Bash(git push *)" "Bash(gh pr merge *)" "Bash(gh issue edit *)" "Bash(gh label create *)"` |

### Dev (Codex)

- **`--full-auto` is gone** from codex 0.160.0 (`error: unexpected argument '--full-auto' found`). The settings it used to imply are now passed as `-c` config overrides: `sandbox_mode="workspace-write"`, `approval_policy="never"` (never ask, so an unattended turn cannot stall), and `sandbox_workspace_write.network_access=true`, which the Dev needs for `git push` and `gh pr create`.
- **`--ignore-user-config`** skips `~/.codex/config.toml`, so a user's config cannot weaken or change the sandbox. Auth still comes from `CODEX_HOME`.
- **`--strict-config`** makes codex reject unknown keys and bad values in `-c` overrides. Without it, a misspelt key is silently ignored. Each key and value above was checked this way: `-c bogus_key=1` fails with `unknown configuration field`, and `approval_policy="bogus"` fails with `expected one of untrusted, on-failure, on-request, granular, never`. All valid values pass.
- **Go caches.** The workspace-write sandbox can write only the workdir, `/tmp` and `$TMPDIR`, so `go test` could not write the module or build cache. `runner.GoCacheDirs` asks `go env GOMODCACHE GOCACHE` for the paths (creating them if missing). They are passed as `--add-dir`, which the help describes as "Additional directories that should be writable alongside the primary workspace".

### PM (Claude Code)

- **`--permission-mode dontAsk`** replaces `acceptEdits`. It "auto-denies every call that would otherwise prompt". File reads in the working directory and other approval-free actions still run, as do tools matched by an allow rule. `acceptEdits` would have let the PM edit any file and run `mkdir`, `touch`, `mv` and `cp`.
- **`Edit(/Session.md)`** is the only edit permission. Per the docs, `Edit` rules apply to every built-in file-editing tool, including Write. A path rule written for `Write(...)` is accepted but never consulted. A leading `/` anchors the path at the primary working directory for CLI rules, so only the repo root's `Session.md` matches.
- **Bash rules** use the documented `Bash(cmd *)` form. The older `Bash(cmd:*)` form is equivalent. A trailing ` *` also matches the bare command (`git status`). Compound commands are split on `&&`, `;`, `|` and the like, and each part must match a rule on its own.
- **`--disallowedTools`** explicitly denies `git push`, `gh pr merge`, `gh issue edit` and `gh label create`. `dontAsk` would refuse them anyway. The explicit deny also beats any allow rule the target repo's own `.claude/settings.json` might add, because deny is evaluated first at every settings level. Per the docs, a Bash deny rule matches the command as written, so it does not stop `git -C . push` or `sh -c 'git push'`. Those are still refused by `dontAsk`, because no allow rule matches them.
- **Both prompts go on stdin**, never in argv. This avoids argument length limits (`codex exec -` reads stdin; `claude -p` with no prompt argument does too). It also matters because `--allowedTools` and `--disallowedTools` are variadic, so a prompt placed after them would be read as another rule.

### Claude Code JSON envelope

With `--output-format json`, stdout is a single JSON object. The answer text is the **`result`** field. Other useful fields: `"type": "result"`, `"subtype": "success"`, `"is_error": false`, and `permission_denials` (a list of refused tool calls). The envelope also includes `total_cost_usd` (about $0.05 for the smoke test). That is a notional list-price figure (`"costBasis": "list"`). The run used the claude.ai subscription login with no API key in the environment, so nothing was billed.

### Smoke test

`TestSmokeRealAgents` runs each real CLI through the runner with the prompt "Reply with the word OK". It also runs Codex once more with a prompt to run `go test ./...` inside its sandbox. It is skipped unless `HUDDLE_SMOKE` names a target clone:

```bash
HUDDLE_SMOKE=~/projects/huddle-sandbox go test -count=1 -run Smoke -v ./internal/runner
```

Result on 2026-10-12 in `~/projects/huddle-sandbox`, with the commands above: all three passed.

- Codex: exit 0 in 12 s, answered `OK`. Its header, with `~/.codex/config.toml` ignored, reported `approval: never` and `sandbox: workspace-write [workdir, /tmp, $TMPDIR, /home/sweeng/go/pkg/mod, /home/sweeng/.cache/go-build] (network access enabled)`. Following `AGENTS.md`, it read `Session.md` and appended a line to it, even for this trivial prompt.
- Codex `go test`: exit 0 in 13 s. `go test ./...` passed inside the sandbox, and it answered `OK`.
- Claude: exit 0 in 5 s, envelope `result` = `"OK"`, `is_error` false, `permission_denials` empty. This prompt uses no tools, so it shows that the flags parse, not that the rules behave as intended. The rules were checked against the docs only.
