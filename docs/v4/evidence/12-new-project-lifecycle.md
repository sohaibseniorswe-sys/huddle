# New-project lifecycle and fresh-machine acceptance

2026-10-17, real GitHub and the canonical v4 binary, isolated roots. The single
private acceptance repository was authorized by the PM in 6497984:
[sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427).
It is archived after acceptance. No other new repository was created.

The initial run created it from an unnamed internal identity, resumed a saved
planning package, approved creation/publishing, reached done and published N+1.
Review then found that the harness had supplied required agent instruction files.
Bootstrap was corrected to supply its own non-overwriting local starters. A
second run reused the **same** owned private repository (temporarily unarchived),
with a new isolated root, proving repository recovery and the corrected bootstrap
without another repo creation. It asserts startup files exist before the harness
runs. The follow-up first hit a duplicate fixture milestone title (HTTP 422). With a
unique title, the same root resumed its completed setup and passed in 47.67s.
The repo was archived again afterwards.

These runs use scripted approval and scripted Dev/PM worker processes. The real
binary, process runner, remote APIs, project clone, build lease, durable stores,
phase routing and fresh-root listing execute normally. Live-model read-only
planning evidence is recorded separately in [07-planning.md](07-planning.md).

## Observations

1. `planning`: stable id `new-project-acceptance`, initially no project attributes.
   Save/reload resumes exact specs and destination; no directory before approval.
2. `published`: approved private repo/bootstrap precedes specs, labels, milestone,
   ordered implementation issue and dev-ready control. Opening refreshes GitHub.
3. `building`: canonical binary starts fresh Dev, which pulls and verifies specs;
   fresh PM holds a real build lease. N+1 planning proceeds concurrently, and its
   publish is refused while N's control remains active.
4. `done`: PM marks/closes N's control; lifecycle refresh derives done. Entering
   plan-next retains the N+1 draft, then its approval publishes the next control.
5. Fresh machine: a separate root clones committed huddle-data and `--list` prints
   both stable ids and the correct then-current PLANNED phase/status without
   opening either control. Fixture cleanup follows this listing.

The initial [control #2](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/2)
and [N+1 control #4](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/4)
retain the first lifecycle history. Exact specifications remain on
[the acceptance branch](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/tree/v4-acceptance/docs/sandbox-lifecycle-fixture).

## Commands and result

See [09-new-project.md](09-new-project.md) for the initial creation command. The
follow-up used the same repo and `HUDDLE_V4_NEW_HOME=/tmp/huddle-v4-new-20261017-0427-final`.
Do not rerun with another name: this acceptance authorizes one repository only.

Follow-up controls: [N #6](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/6)
and [N+1 #8](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/8).
The second run recovered the owned repository created by the first run; it does
not claim another new-repository creation.

```text
=== RUN   TestNewProjectLifecycle
    newproject_test.go:101: REAL mid-planning resume: id=new-project-acceptance phase=planning name=New project acceptance dir=/tmp/huddle-v4-new-20261017-0427-final/workspace repo=sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427; proposed destination={Name:New project acceptance Dir:/tmp/huddle-v4-new-20261017-0427-final/workspace Repo:sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427 Branch:v4-acceptance}
    newproject_test.go:112: REAL first publish result: {Repo:sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427 Branch:v4-acceptance Milestone:3 Title:New project acceptance huddle-v4-new-20261017-0427-final Issues:map[01-response:5] Control:6} (error=<nil>); isolated root=/tmp/huddle-v4-new-20261017-0427-final
    lifecycle_test.go:120: REAL sandbox binary build loop (scripted Dev/PM):
        new-project-acceptance: start published milestone
        ✔ git repo /tmp/huddle-v4-new-20261017-0427-final/workspace, remote sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427
        ✔ AGENTS.md, CLAUDE.md and Session.md present
        ✔ gh logged in, repo access ok, labels ready
        ✔ settings /tmp/huddle-v4-new-20261017-0427-final/huddle-data/config/new-project-acceptance.yaml
        ✔ Dev: Codex · CLI default model · CLI default effort; PM: Claude Code · CLI default model · CLI default effort; companion: Claude Code · CLI default model · CLI default effort
        ✔ autonomy 8 · Autonomous
        ✔ codex and claude installed with subscription logins, no API keys in env
        ✔ control issue #6 "New project acceptance huddle-v4-new-20261017-0427-final · Control", milestone "New project acceptance huddle-v4-new-20261017-0427-final", state: dev-ready
        huddle: turn 1: Dev (Codex), fresh session
        From https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427
           ef6dc27..afaa73b  v4-acceptance -> origin/v4-acceptance
        ✓ Closed issue sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427#5 (Provide a fixture response)
        session id: lifecycle-dev
        READY
        huddle: turn 1: Dev (Codex) exited with code 0
        huddle: Dev (Codex) set state:pm-ready
        huddle: turn 2: PM (Claude Code), fresh session
        ✓ Closed issue sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427#6 (New project acceptance huddle-v4-new-20261017-0427-final · Control)
        {"type":"result","subtype":"success","session_id":"lifecycle-pm","result":"Sandbox lifecycle completed."}
        huddle: turn 2: PM (Claude Code) exited with code 0
        huddle: PM (Claude Code) set state:done
        huddle: state:done: the milestone is finished. Final report:
        huddle: --- sohaibseniorswe-sys https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/6#issuecomment-6229674475
        Status: Completed. Sandbox fixture issue verified; lifecycle complete.
        ---
        
        Agent args:
        exec --strict-config --ignore-user-config -c sandbox_mode="danger-full-access" -c approval_policy="never" --output-last-message /tmp/huddle-v4-new-20261017-0427-final/logs/20261017-003253-turn1-dev.last -
        -p --output-format json --dangerously-skip-permissions
    newproject_test.go:174: REAL full new→plan→publish→build→done→plan-next: first control https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/6; next control https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/8; specs https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/tree/v4-acceptance/docs/sandbox-lifecycle-fixture
    newproject_test.go:189: REAL fresh-machine clone/list before cleanup:
        New project acceptance · PLANNED · published [new-project-acceptance]
        Untitled project · PLANNED · planning [second-planning-project]
    newproject_test.go:108: Archived private acceptance repository sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427
--- PASS: TestNewProjectLifecycle (47.67s)
PASS
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	47.676s
```

The final metadata read reported `isPrivate: true, isArchived: true`. Both
milestones' fixture work/control issues were closed before archival. Logs and
isolated roots are retained on the acceptance host; the repository links and
committed evidence remain the portable record.
