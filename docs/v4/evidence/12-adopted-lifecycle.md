# Adopted-project lifecycle and fresh-machine evidence

2026-10-17 00:11–00:12 EDT. Real GitHub, compiled v4 binary, OS build lease and
process runner, isolated state roots and a temporary huddle-sandbox checkout.
Dev/PM processes were scripted acceptance fixtures; real companion planning is
recorded separately in [07-planning.md](07-planning.md).

This run proves the **existing-project** lifecycle. The subsequently completed
new-project acceptance is recorded separately in [12-new-project-lifecycle.md](12-new-project-lifecycle.md).

The run adopted the remote, normalized an old issue into validated specs,
published, ran fresh Dev/PM processes through the real build loop, drafted N+1
while PM was live, refused premature publish, reached done, resumed N+1 and
published it. The scripted Dev owned checkout synchronization and verified that
the published README existed locally. The scripted PM closed the sandbox control
with done. No live Huddle control was used.

A second fresh state root cloned the committed huddle-data store and listed both
stable identities with their then-current cached PLANNED statuses. Cleanup later
closed fixture issues/milestones; the evidence branch is retained.

Command: `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run '^TestSandboxLifecycleFromAdoptedProject$' -v`

```text
=== RUN   TestSandboxLifecycleFromAdoptedProject
    lifecycle_test.go:117: REAL sandbox binary build loop (scripted Dev/PM):
        fe42bfb4bf7533f346ceec97f2e22810: start published milestone
        ✔ git repo /tmp/TestSandboxLifecycleFromAdoptedProject246928969/002/existing, remote sohaibseniorswe-sys/huddle-sandbox
        ✔ AGENTS.md and CLAUDE.md present, created empty Session.md
        ✔ gh logged in, repo access ok, labels ready
        ✔ settings /tmp/TestSandboxLifecycleFromAdoptedProject246928969/001/huddle-data/config/fe42bfb4bf7533f346ceec97f2e22810.yaml
        ✔ Dev: Codex · CLI default model · CLI default effort; PM: Claude Code · CLI default model · CLI default effort; companion: Claude Code · CLI default model · CLI default effort
        ✔ autonomy 8 · Autonomous
        ✔ codex and claude installed with subscription logins, no API keys in env
        ✔ control issue #68 "v4 publish evidence 20261017-041128 · Control", milestone "v4 publish evidence 20261017-041128", state: dev-ready
        huddle: turn 1: Dev (Codex), fresh session
        From https://github.com/sohaibseniorswe-sys/huddle-sandbox
           c91d2a3..8975757  v4-publish-evidence-20261017-041128 -> origin/v4-publish-evidence-20261017-041128
        ✓ Closed issue sohaibseniorswe-sys/huddle-sandbox#67 (Provide a fixture response)
        session id: lifecycle-dev
        READY
        huddle: turn 1: Dev (Codex) exited with code 0
        huddle: Dev (Codex) set state:pm-ready
        huddle: turn 2: PM (Claude Code), fresh session
        ✓ Closed issue sohaibseniorswe-sys/huddle-sandbox#68 (v4 publish evidence 20261017-041128 · Control)
        {"type":"result","subtype":"success","session_id":"lifecycle-pm","result":"Sandbox lifecycle completed."}
        huddle: turn 2: PM (Claude Code) exited with code 0
        huddle: PM (Claude Code) set state:done
        huddle: state:done: the milestone is finished. Final report:
        huddle: --- sohaibseniorswe-sys https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/68#issuecomment-6229490171
        Status: Completed. Sandbox fixture issue verified; lifecycle complete.
        ---
        
        Agent args:
        exec --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message /tmp/TestSandboxLifecycleFromAdoptedProject246928969/001/logs/20261017-001154-turn1-dev.last -
        -p --output-format json --dangerously-skip-permissions
    publish_test.go:260: REAL fresh-machine clone → list:
        existing · PLANNED · published [fe42bfb4bf7533f346ceec97f2e22810]
        Untitled fresh project · PLANNED · planning [fresh-planning]
    publish_test.go:262: REAL onboarding: remote adopted into stable id fe42bfb4bf7533f346ceec97f2e22810; normalized issue #66; N+1 drafted while PM-working and gated; done-project entry resumed N+1; then published control https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/70.
    publish_test.go:264: REAL sandbox publish: milestone https://github.com/sohaibseniorswe-sys/huddle-sandbox/milestone/20; control https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/68; implementation issue map[01-response:67]; superseded #66 closed and linked; labels ready; exact specs at https://github.com/sohaibseniorswe-sys/huddle-sandbox/tree/v4-publish-evidence-20261017-041128/docs/v4-publish-20261017-041128; second active publish refused without branch changes.
--- PASS: TestSandboxLifecycleFromAdoptedProject (57.50s)
PASS
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	57.508s
```
