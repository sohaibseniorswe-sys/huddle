# Huddle

Huddle lets two AI coding agents work through a GitHub milestone in turns: Codex CLI as the Dev agent, Claude Code as the PM agent. The human steps in only when the agents escalate. The spec is [MVP_PLAN.md](MVP_PLAN.md).

**Status:** Milestone 1 (skeleton and startup checks). The turn loop is not implemented yet. The full README (install, usage, labels, troubleshooting) comes in Milestone 5.4.

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
| Claude Code | `2.1.288` | `claude auth status --json` | JSON with `"loggedIn": true`, `"authMethod": "claude.ai"`, `"apiProvider": "firstParty"`, `"subscriptionType": "pro"` | `loggedIn` true, `authMethod` = `claude.ai`, `apiProvider` = `firstParty` |

Notes:

- `codex login --with-api-key` exists. Its status text was not observed, but it does not say "ChatGPT", so it is rejected.
- `claude auth status` defaults to JSON; `--text` prints e.g. `Login method: Claude Pro account`. Huddle parses the JSON.
- Requiring `apiProvider` = `firstParty` also rejects Bedrock and Vertex setups, which bill through a cloud account.
- A `claude setup-token` login has not been observed. If its `authMethod` is not `claude.ai`, Huddle rejects it (see Decisions).
