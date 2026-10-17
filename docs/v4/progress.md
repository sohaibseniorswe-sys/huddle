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
