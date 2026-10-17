# 02 — Project identity & lifecycle-phase derivation

## Objective
Define the project identity model and derive lifecycle phase/status from GitHub.

## Context
Projects must be representable before they have a repo, and a project's status can't be a GitHub label (milestones have none). See README §2–3.

## Requirements
- Project keyed by **internal id**; `repo` and `dir` are optional attributes. A planning-only project has neither.
- **Phase derivation** from GitHub: `planning` (draft only, no control issue) / `published` (control at `state:dev-ready`, not started) / `building` (control in a `*-working`/`*-ready`/`human-required` state) / `done` (control `state:done` or milestone closed).
- **Status** PLANNED / ACTIVE / DONE derived from milestone open/closed + control-issue state; cached in the registry; the control issue is re-read on open and wins over the cache.

## Out of scope
Startup UI and routing (issue 03).

## Success criteria
- Phase computed correctly for each fixture GitHub state.
- An id-only (no repo) project is representable.
- When cache and re-read disagree, the re-read value is used.

## Evidence required
Table-driven tests over fixture control-issue/milestone/label states.

## Dependencies
01
