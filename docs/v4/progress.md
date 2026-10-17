# v4 progress

One entry per issue: start/finish time, what was built, deviations from the spec, anything that needs the human.

---

## v4.01 — #13 state root, store and registry

- Start: 2026-10-16 20:27 EDT. Finish: 2026-10-16 20:29 EDT.
- Built: `--home` / `HUDDLE_HOME` resolution, root-relative logs/sessions/queues,
  Huddle-owned Git store initialization/cloning, per-ID project records and light
  index, immediate config/registry commits, explicit draft checkpoints, and
  byte-preserving legacy migration with backups and idempotent retries.
- Evidence: temp-root isolation/registry tests; exact-byte migration and retry
  tests; fresh-root local clone test; full Go tests, vet and binary build.
- Technical choice: legacy repos get deterministic initial IDs; new projects can
  use random IDs independent of optional name, directory or repo. OS-home display
  and wizard tilde expansion remain distinct from the state root.
- Deviations: none. Human needed: none.

## v4.02 — #14 identity and lifecycle derivation

- Start: 2026-10-16 20:30 EDT. Finish: 2026-10-16 20:31 EDT.
- Built: typed lifecycle snapshots and phase/status derivation, active-control
  discovery, fresh label reads, authoritative cache refresh, optional repo/dir
  identity, and detection of multiple active controls.
- Evidence: fixtures cover every ready/working/human/done state, closed milestone,
  closed-control drift, id-only planning, cache disagreement and read failure.
  Full Go suite, vet and binary build pass.
- Assumption: `dev-ready` is published until a recorded launch/handoff or a
  previously persisted start; later Dev handoffs stay building. An absent known
  milestone in GitHub's complete open-milestone list means it is closed.
- Deviations: none. Human needed: none.

## v4.03 — #15 registry startup and routing

- Start: 2026-10-16 20:31 EDT. Finish: 2026-10-16 20:33 EDT.
- Built: local registry picker, cached status listing, `--list`, `--project`,
  `--new`, phase routing and completed-project plan-next/archive choice. A new
  unnamed identity opens planning without a repository or checkout. Planning
  memory keys use internal IDs so unnamed projects do not share conversations.
- Before: an omitted `--repo` selected a hardcoded repository. After: no target
  opens the registry picker; repo-backed projects all use the same generic path.
  No project-specific references remain in executable internal/cmd code.
- Evidence: listing/route/picker tests, new-project reopen test without GitHub or
  checkout, existing v3 direct-entry tests; full suite, vet and v4 binary build.
- Technical choice: `--repo` retains the explicit generic v3 entry; normal v4
  entry is registry-first. Planning/publish internals follow in #19/#20.
- Deviations: none. Human needed: none.

## v4.04 — #16 resume and reconcile

- Start: 2026-10-16 20:33 EDT. Finish: 2026-10-16 20:35 EDT.
- Built: stale-directory detection, relocate/drop prompt and persisted repair;
  project-scoped abandoned-draft pruning; closed-control lifecycle reconciliation
  consistent with existing done rules. Registry directory repair overrides stale
  settings paths while the ordinary Git remote check still validates the repo.
- Fixture evidence: moved/deleted/non-directory path, invalid relocation, durable
  relocation and drop; abandoned draft reopen/prune; closed controls with closed
  vs reopened work; existing startup ask and in-loop reopen coverage.
- REAL sandbox live-loop evidence: https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/40
  (`HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run TestSandboxResume -v`).
  Compiled binary, temporary sandbox clone and isolated state root, real GitHub
  control at pm-working, stale published cache; resumed the PM with fresh args
  `-p --output-format json --dangerously-skip-permissions`, then observed Dev-ready.
  Agent processes were scripted fixtures, not live models. Throwaway control and
  milestone closed by cleanup. Full suite, vet and binary build pass.
- Assumption: explicit state:done retains the pre-existing done-rule precedence;
  closing without done requires all milestone work closed, otherwise reopening.
- Deviations: none. Human needed: none.

## v4.05 — #17 fresh issue sessions

- Start: 2026-10-16 20:35 EDT. Finish: 2026-10-16 20:39 EDT.
- Built: v4 per-issue prompt boundary, fresh Dev/PM turn sessions, durable-source
  reload instructions, and persisted building/start state before each turn.
  Same-role Dev-ready continues autonomous work in a new process; usage retries
  within a turn retain the current issue session. Legacy direct-entry v3 policy
  remains unchanged.
- Evidence: unit tests for repeated same-role boundaries, stale saved session IDs,
  PM freshness and usage retry. REAL multi-issue sandbox run (scripted agents):
  https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/47
  `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run TestSandboxFreshIssueSessions -v`.
  Separate Claude PM/Codex Dev processes reloaded control+comments, issue list,
  agent files, Session.md and git history before closing their fixture issues.
  Args contain neither `resume` nor `--resume`; roots/clones are temporary and
  cleanup closes all fixture issues/milestone. Full tests, vet and build pass.
- Before/after context note: previously a saved role session carried all earlier
  issue transcripts; v4 boundary args pass zero prior-session transcript, only
  the fixed launch prompt plus freshly read durable sources. This is structural
  context isolation, not a model-token benchmark.
- Technical choice: one implementation issue per Dev turn; same-role ready is a
  session boundary rather than a mandatory PM review. Agent CLIs were scripted
  in sandbox evidence; the process runner/GitHub/loop were real.
- Deviations: none. Human needed: none.

## v4.06 — #18 concurrency lanes

- Start: 2026-10-16 20:39 EDT. Finish: 2026-10-16 20:42 EDT.
- Built: root-scoped OS build lease across processes, clear second-build refusal,
  crash-safe lock release, independent unrestricted companion lane, and dynamic
  shared-CLI usage warning. Git store commits are serialized separately; config
  and registry commits no longer checkpoint unfinished planning drafts.
- Evidence: lease/refusal/isolation/warning tests; killed-subprocess lock recovery;
  concurrent registry/index commits; draft-checkpoint separation. REAL sandbox:
  https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/50
  `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run TestSandboxConcurrentPlanning -v`.
  Real binary/loop/GitHub with scripted agents: PM X held running, second build
  refused, companion Y answered with read-only args (`--setting-sources user`),
  usage warning fired, then PM X handed off normally. Fixture cleanup completed.
- Validation: full Go tests, vet, v4 binary build and Windows cross-compile.
  Race checks could not run: CGO is disabled and this image has no C compiler. The temporary cross-compile artifact was discarded and the
  canonical Linux test binary remains `./bin/huddle-v4`.
- Technical choice: locks are per state root to preserve deliberate instance
  isolation; metadata alone never establishes liveness.
- Deviations: Windows cross-compile used a temporary output artifact, removed
  after checking; no production binary touched. Human needed: none.

## v4.07 — #19 companion-driven planning drafts

- Start: 2026-10-16 20:43 EDT. Finish: 2026-10-16 20:48 EDT.
- Built: embedded Milestone Spec Standard, typed JSON draft package, required
  section/process-language/dependency checks, exact reviewable Markdown specs,
  portable planning history, response checkpoints and durable context reload.
  Malformed candidates also persist for repair. Removed specs leave no stale
  rendered files. V4 rejects the old direct plan-publish protocol.
- Tests: good/bad fixtures (missing/empty sections, process instructions, unknown
  dependencies, duplicate deps, cycles, unsafe IDs), stable dependency order,
  clone-to-fresh-root conversation/draft resume, and app no-GitHub-write checks.
- REAL sandbox planning: a real Claude read-only companion returned a valid
  one-issue spec package, saved/resumed successfully; sandbox checkout unchanged;
  process logs verified `--setting-sources user`. Transcript in
  `docs/v4/evidence/07-planning.md`; opt-in reproduction command there.
- Validation: full Go tests, vet, v4 build. No new external dependencies.
- Technical choice: checkpoint whole responses, retaining incomplete drafts for
  editing; only publish will require a clean validation result.
- Deviations: none. Human needed: none.

## v4.08 — #20 approved publishing

- Start: 2026-10-16 20:48 EDT; resumed 23:48 EDT after the CLI usage wait.
  Finish: 2026-10-16 23:52 EDT.
- Built: exact-revision/destination approval, `/publish` review/Agree card,
  validation and active-control gate before writes, Git Data API spec commits to
  the named working branch (no project Git mutation), missing labels, milestone,
  dependency-ordered short-reference issues, clean dev-ready control, adoption
  replacement links/closure, and journaled retry after partial failures.
- Evidence: approval/stale-review/no-write/gate/failure/retry/dependency/adoption
  fixtures and app approval tests. REAL sandbox publish created milestone 15,
  implementation #52 and control #53; superseded #51 closed with replacement
  link; exact README/specs verified on the retained evidence branch; second
  approved publish refused without changing the branch. All fixture issues and
  milestone closed in cleanup. See `docs/v4/evidence/08-publish.md`.
- Validation: full Go tests, vet, v4 binary build and diff check.
- Technical choices: approval covers content plus repo/branch; remote branch
  updates are fast-forward only. Publishing does not implicitly start building.
- Deviations: none. Human needed: none.

## v4.10 — #22 existing-project onboarding

- Start: 2026-10-16 23:52 EDT. Finish: 2026-10-16 23:58 EDT.
- Built: picker/CLI adoption by remote or directory, durable open-issue input for
  re-spec vs fresh planning, ID-based settings for repo-less identities,
  `--plan-next`, independent planning while building, archive of a published
  draft and restart-safe resume of unfinished N+1. Planning now bypasses build
  preparation, project file/label writes and build-agent login requirements.
- Evidence: identity/directory fallback, no-write planning, published draft archive,
  runtime preservation and unfinished-draft resume tests. REAL sandbox adoption:
  remote adopted into ID 11e2c2a4388d1628d230fbb91458f232; #54 normalized/linked
  to #55; N control #56 at PM-working while N+1 drafted and publish was refused;
  done entry resumed N+1 and then published #58. See
  `docs/v4/evidence/10-onboarding.md`. Fixtures closed; evidence branch retained.
- Validation: full Go tests, vet, v4 binary build and diff check.
- Technical choice: `--adopt --respec` chooses normalization; plain `--adopt`
  chooses a fresh milestone. Current runtime phase/control are never overwritten
  merely to begin planning.
- Deviation/dependency note: #21's new-directory/repo creation evidence is blocked
  on the sandbox-only/Git ownership conventions; question recorded on #21.
  Existing-repo onboarding has no dependency on that flow. Human needed: none;
  the PM can decide the #21 convention conflict at handoff.

## v4.11 — #23 selectable/copyable agent output

- Start: 2026-10-16 23:58 EDT. Finish: 2026-10-17 00:03 EDT.
- Built: in-app raw-log/companion selection in content-row/cell coordinates,
  forward/backward dragging, stationary edge autoscroll, retained selection during
  mouse/keyboard scrolling, themed highlight, Unicode-safe plain-text copy via
  OSC 52, Ctrl+C copy while selected, Ctrl+Y alternative and Escape clearing.
  Mouse-wheel/activity/card behavior remains available; resize/source switches
  clear stale coordinates. Terminal-reserved Ctrl+Shift+C behavior documented.
- Evidence: unit tests for selection/copy/scroll/Unicode/companion/source changes.
  Controlled PTY verification through the real renderer selected 18 rows downward
  and 12 backward/upward, decoded both OSC 52 payloads, verified highlight color,
  and exited cleanly after clearing. Notes: `docs/v4/evidence/11-text-selection.md`.
  The first interactive fixture needed correct quit wiring; fixed and the final
  controlled replay passed. The normal v4 binary was restored after the demo.
- Validation: full Go suite, vet, canonical v4 build and diff check.
- Trade-off: raw/companion drag selects in-app; native terminal selection remains
  Shift-drag. Clipboard requires terminal OSC 52 support; desktop paste is not
  available in this headless session. Selection spans the loaded log window and
  resets when a different window/source is loaded.
- Deviations: none. Human needed: none.

## v4.09 — #21 convention conflict, not completed

- Start: 2026-10-16 23:52 EDT; turn checkpoint: 2026-10-17 00:16 EDT.
- The new unnamed identity and durable planning foundation are implemented in
  #15/#19. Remaining name/directory/repo output creation is not implemented.
- Blocker: #21's real new-repo walkthrough conflicts with AGENTS.md's prescribed
  huddle-sandbox-only real runs; the README also reserves project Git operations
  to agents while a new build needs an initialized checkout. Decision request:
  https://github.com/sohaibseniorswe-sys/huddle/issues/21#issuecomment-6229298323
- The PM should resolve the acceptance scope and bootstrap ownership. An
  agent-owned bootstrap is an option to preserve the Git invariant; new-repo
  creation should remain within the human-approved publish write path.
- No extra repository or unapproved project checkout operation was performed.
  Human needed: none at this point; PM decision required. Issue remains open.

## v4.12 — #24 integration checkpoint, not completed

- Start: 2026-10-17 00:04 EDT; turn checkpoint: 00:16 EDT. Final finish pending #21.
- Built: adopted-project publish → real binary Dev/PM loop → done → plan-next
  integration harness, fresh-root clone/list proof, current operating guide and
  README/CLI help. Audit fixes: all-page GraphQL issue/control discovery and
  milestone work reads (no 200/1000 truncation), and planning autonomy override
  applied without persisting the run-only setting. A REST pagination prototype
  exposed a fresh-read timing failure and was discarded in favor of the existing
  GraphQL semantics with pagination.
- REAL sandbox evidence: normalized #66 → #67, N control #68, N+1 control #70;
  N+1 drafted/gated while a real scripted PM process held the build lease; Dev
  and PM started fresh, PM reached done; a fresh clone listed two stable IDs and
  correct then-current cached statuses. Fixtures closed, evidence branch retained.
  Details: `docs/v4/evidence/12-adopted-lifecycle.md`.
- Regression: full tests/vet/gofmt/canonical build pass; v3 direct-entry/session/
  prompt/golden checks remain green. PRs #26–#35 passed Linux/macOS CI.
- Correction to v4.06: full-app Windows cross-compilation did NOT pass; the base
  runner already uses Unix-only Setpgid/syscall.Kill. Only the new lock package
  compiles for Windows. Earlier progress/PR wording overstated that check; PR #31
  metadata has been corrected. Linux binary restored. Race unavailable: no C
  compiler. See `docs/v4/evidence/12-regression.md`.
- Remaining: implement/evidence #21's complete from-scratch flow, then finish the
  exact new → plan → publish → build → done → plan-next acceptance and close #24.
  An adopted-project run is not claimed as new-repo evidence. Human needed: none;
  handoff to PM for the recorded convention conflict.

## v4.09 resumed — 2026-10-17 04:24 UTC
Start: PM decision 6497984 authorizes the single private throwaway acceptance repo and bootstrap. Implementing proposed project outputs within the existing publish gate; choosing the documented setup-only clone exception. No human blocker.

Finish: 2026-10-17 04:30 UTC. Built optional typed new-project draft outputs,
registry name caching, exact destination review, approved private gh/API creation,
setup-only staged clone and durable destination/step recovery. Preserved config
choices while binding repo/path to the stable id. Existing/foreign destinations
are refused; partial setup cannot redirect. README documents the sole clone
exception selected under the PM decision. New tests cover success and failures.
REAL authorized acceptance: private `huddle-sandbox-new-20261017-0427`, isolated
root `/tmp/huddle-v4-new-20261017-0427`; mid-planning reload → approved creation →
publish → real binary with scripted Dev/PM → done → plan-next → fresh-root clone/
list passed in 55.64s. Repository archived afterwards. No extra repo created.
Evidence: `docs/v4/evidence/09-new-project.md`; #24 final writeup follows.
Validation: full Go tests, vet and canonical build passed; final CI pending.
Deviations: bootstrap uses the PM-permitted one-time clone, explicitly documented.
Human needed: none.

v4.09 verification correction (04:33 UTC): review found that the original
lifecycle harness supplied AGENTS.md/CLAUDE.md, masking a first-Start precondition.
Bootstrap now creates non-overwriting local instruction starters without Git
commits. The harness requires them before launch and preserves them. A follow-up
uses the same private repo and a second isolated root, including recovery of the
already-created remote. Its first attempt hit a duplicate fixture milestone title
(HTTP 422); the fixture now uses a unique title, and retry retains the completed
setup. No second repo was created. Final result recorded below.


## v4.12 final acceptance — #24
Start: 2026-10-17 04:33 UTC. #21 implementation merged in #37 (c518e52), with
Linux/macOS CI green. Finishing lifecycle/fresh-machine notes and legacy proof.
Finish: 2026-10-17 04:35 UTC. REAL corrected bootstrap run reused the single
private repo `huddle-sandbox-new-20261017-0427`, resumed the completed setup after
a duplicate fixture milestone title was corrected, and passed the full lifecycle
in 47.67s. New bootstrap instruction starters existed before launch; the harness
preserved them. Controls #6 → done and #8 N+1; fresh clone listed both stable ids
with correct then-current statuses. Repo archived again; no additional repo.
Evidence: `12-new-project-lifecycle.md`, with the v4.09 masking correction added
to `09-new-project.md`. Prior adopted evidence remains separately identified.
REAL legacy direct `--repo --resume` sandbox run: PM session preserved, actual
Claude argv contained `--resume legacy-pm-checkpoint`; control #72 handed back to
dev-ready, fixture closed. `12-regression.md` contains output; listing regression
includes Palantir as an ordinary cached registry entry, without a live run.
User guide/README reflect implemented flow; outdated acceptance blocker removed.
Full Go tests, vet, gofmt/diff checks and canonical build run on the final tree;
Linux/macOS PR CI must pass before merge. Existing limits remain: no C compiler
for race, full Windows app unsupported by the pre-existing Unix-only runner,
headless terminal clipboard transport verified without a desktop paste target.
Human needed: none. Dev will leave #25 open for PM final review.
