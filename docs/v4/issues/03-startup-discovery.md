# 03 — Startup discovery, listing & routing

## Objective
On launch, read the registry, list managed projects with derived status, route by phase, and remove the hardcoded Palantir entry point.

## Context
Removing the Palantir special-case is the point of v4's startup. See README §3.

## Requirements
- On launch, read the registry and list managed projects with derived status. This is Huddle info and must be cheap; **do not** compute any project's next steps while listing.
- Open a project or start a new one; **route by phase**: planning → resume planning; published → offer to start; building → resume (reconcile first, issue 04); done → offer plan-next or archive.
- Remove any Palantir-specific entry/code path; Palantir becomes a normal registry entry reached through the generic flow.

## Out of scope
Resume/reconcile internals (04); planning (07); publish (08).

## Success criteria
- A seeded registry lists with correct per-project status.
- Routing selects the correct branch for each phase (fixtures).
- No Palantir-specific code path remains (grep clean); Palantir opens via the generic path.

## Evidence required
Tests for listing and routing; a short before/after note showing the Palantir special-case removed.

## Dependencies
02
