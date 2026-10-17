# Regression result

2026-10-17. `go test ./...`, `go vet ./...`, `gofmt -l .` and
`go build -o ./bin/huddle-v4 ./cmd/huddle` passed. The suite retains the v3 direct
entry, prompt, loop/session and golden-screen checks alongside v4 coverage.
Feature PRs #26–#37 passed Linux and macOS CI. Final #24 CI is recorded on its PR.

Correction to the earlier v4.06 Windows note: the **full application** does not
cross-compile for Windows because the pre-existing process runner uses Unix-only
`Setpgid` and `syscall.Kill` (also present at base commit 53bd4cc). This is not a
new v4 regression. The new file-lock package compiles separately for Windows.
No claim of native Windows application support is made. The canonical Linux
binary was rebuilt after that check. Race checks are unavailable without a C
compiler in this image.

```text
ok  	github.com/sohaibseniorswe-sys/huddle/cmd/huddle	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/activity	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/agents	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/app	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/checks	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/commands	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/companion	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/config	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/filelock	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/gh	(cached)
?   	github.com/sohaibseniorswe-sys/huddle/internal/gitx	[no test files]
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/lanes	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/lifecycle	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/loop	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/onboarding	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/planning	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/prompts	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/publish	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/runner	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/session	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/start	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/state	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/store	(cached)
?   	github.com/sohaibseniorswe-sys/huddle/internal/ui	[no test files]
ok  	github.com/sohaibseniorswe-sys/huddle/internal/wizard	(cached)
```

## Final legacy direct-entry acceptance

A real isolated huddle-sandbox checkout ran the canonical binary through
`--repo sohaibseniorswe-sys/huddle-sandbox --resume --plain --once --yes`.
The scripted PM retained its existing session id, used the real runner/GitHub,
and handed control back to dev-ready. Fixture cleanup closed the control and
milestone. No live project/control was used. This complements the full suite's
v3 startup/prompt/session/golden coverage; it does not claim live-model testing.

Command: `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run '^TestSandboxLegacyResume$' -count=1 -v`

```text
=== RUN   TestSandboxLegacyResume
    resume_test.go:229: REAL sandbox live-loop evidence (scripted PM): https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/72
        target: sohaibseniorswe-sys/huddle-sandbox (override)
        ✔ git repo /tmp/TestSandboxLegacyResume2233771014/001, remote sohaibseniorswe-sys/huddle-sandbox
        ✔ AGENTS.md and CLAUDE.md present, created empty Session.md
        ✔ gh logged in, repo access ok, labels ready
        ✔ settings /tmp/TestSandboxLegacyResume2233771014/002/huddle-data/config/f57e949cf60ad49d15a887e4944e17c2.yaml
        ✔ Dev: Codex · CLI default model · CLI default effort; PM: Claude Code · CLI default model · CLI default effort; companion: Claude Code · CLI default model · CLI default effort
        ✔ autonomy 8 · Autonomous
        ✔ codex and claude installed with subscription logins, no API keys in env
        ✔ control issue #72 "v4 resume evidence 20261017-043346 control", milestone "v4 resume evidence 20261017-043346", state: pm-working
        huddle: state:pm-working found: a previous Huddle run was interrupted; restarting the PM (Claude Code)
        huddle: turn 1: PM (Claude Code), resuming session legacy-pm-checkpoint
        {"type":"result","subtype":"success","session_id":"v4-resume-pm","result":"Resumed the PM turn from durable sandbox state."}
        huddle: turn 1: PM (Claude Code) exited with code 0
        huddle: PM (Claude Code) set state:dev-ready
        huddle: --once: turn finished, exiting

        Agent args: -p --output-format json --dangerously-skip-permissions --resume legacy-pm-checkpoint
--- PASS: TestSandboxLegacyResume (10.81s)
PASS
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	10.809s
```

The registry listing regression also includes a Palantir record beside an
unrelated project, with its own cached DONE phase/status and stable id. Listing
uses the same generic path and makes no GitHub calls; no Palantir build was
launched. Palantir remains a normal existing project to adopt/migrate, with no
special production entry point. The original Palantir golden-screen fixtures
remain valid.
