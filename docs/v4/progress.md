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
