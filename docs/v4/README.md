# Huddle v4 — Project Lifecycle & Multi-Project

One spine: Huddle becomes multi-project and lifecycle-aware, and the hardcoded Palantir entry point is removed. On startup Huddle reads its own registry, lists the projects it manages with their state, and routes into the right place for each project's phase. Planning mode is scoped to a project.

Read this README first, then the issue's own spec in `docs/v4/issues/`.

## 1. Memory model

Two memories, split by ownership.

- **Huddle's memory — a private `huddle-data` repo.** One GitHub repo Huddle owns and manages for *itself*, cloned/cached locally under the state root, holding: the **registry** (one file per project + a light index), **per-project config** (the existing yaml: Dev/PM/companion CLI, effort, autonomy), and **per-project planning drafts** (an in-progress planning conversation + draft spec package for a milestone not yet published).
- **Local only** — run **logs** stay under the state root (`<root>/logs`); the `huddle-data` clone is a working cache.
- **The project's memory (its own repo)** — the control issue (`<!-- huddle:control -->` marker + `control` label) and `state:*` labels: the live truth of where a *published* milestone is.

**Rule:** planning drafts live in `huddle-data` until published; building (runtime) state lives in the project repo; config lives in `huddle-data`. On open, Huddle trusts the project repo for runtime state and `huddle-data` for everything the project repo can't hold. The registry caches a last-known phase but re-reads the control issue on open.

Huddle managing its own `huddle-data` repo does **not** break "Huddle never runs git itself" — that invariant is about *project* repos, which the agents own. `huddle-data` is Huddle's own store.

## 2. Project identity

- The registry is keyed by an **internal id**, stable from creation — not `<owner>_<repo>`, because a brand-new project has no repo yet. Repo and directory are optional attributes, populated as decided.
- A brand-new project starts from nothing: a planning session under a new id. Name → directory → repo are outputs of planning.
- An existing project is identified by the git remote (owner/repo), else the directory path.

## 3. Lifecycle phases

`planning → published → building → done → (plan next | archive)`

- **planning** — draft in `huddle-data` only; no project-repo milestone/issues/control yet.
- **published** — milestone + issues + control exist; control at `state:dev-ready`; not started.
- **building** — build loop running or paused; control at a `state:*-working`/`*-ready`/`human-required`.
- **done** — control at `state:done` / milestone closed.

GitHub milestones can't carry labels, so status (PLANNED / ACTIVE / DONE) is **derived** (milestone open/closed + control-issue state), cached in the registry for fast listing.

## 4. Concurrency invariants

- At most **one build loop** at a time. Starting a second build while one is building is refused.
- **Planning is unrestricted** — any project, any time, including while another builds.
- The **PM** (a build agent) and the **companion** (planning agent) are separate lanes; planning project Y never blocks project X's PM.
- **Publish N+1's control issue only when N is done** — planning N+1 is always allowed; publishing its control issue is gated, keeping exactly one active control issue.
- The companion is **read-only** and never writes to GitHub; all GitHub writes go through the human-approved **publish** step.

## 5. Definition of Done — every issue

- All Success criteria in the issue's spec are met.
- Behaviour works through real code, not stubs; state changes persist to the right store (registry / `huddle-data` / the project repo).
- Inputs, outputs and errors are typed and validated; tests cover success **and** failure.
- Real runs, where a criterion requires one, run on **huddle-sandbox** with an **isolated state root** (`HUDDLE_HOME`), never against a live control issue, and are labelled as real.
- Test binaries build to `./bin/huddle-v4`; **never** `go install` (it would overwrite the frozen prod binary).
- Existing v3 behaviour is preserved.
- A `docs/v4/progress.md` entry records start/finish, what was built, deviations, and anything that needs the human.

## 6. Issue index & dependencies

| # | Issue | Depends on |
|---|-------|-----------|
| 01 | State root + huddle-data store + registry | — |
| 02 | Project identity & lifecycle-phase derivation | 01 |
| 03 | Startup discovery, listing & routing (remove Palantir entry) | 02 |
| 04 | Resume interrupted runs & reconcile drift | 03 |
| 05 | Fresh agent session per issue (clear/restart) | 03 |
| 06 | Concurrency: one build loop, lane separation, usage warning | 03 |
| 07 | Planning mode: companion-driven drafting | 02 |
| 08 | Publish: draft → GitHub, gating, labels | 07, 02 |
| 09 | New project from scratch (Case 2) | 07, 08 |
| 10 | Onboarding flows: adopt, plan-next, plan-while-building | 07, 08 |
| 11 | Selectable & copyable text in agent panes | — |
| 12 | Integration, docs & fresh-machine story | all |

Issue numbers equal a valid topological order, so sequential execution respects dependencies.

## 7. Completion

Milestone complete when every issue meets its Success criteria and the shared Definition of Done, the full lifecycle runs on sandbox (new → plan → publish → build → done → plan-next), the fresh-machine story works (clone `huddle-data` → all projects listed), Palantir runs as a normal registry entry, and v3 milestones still run.
