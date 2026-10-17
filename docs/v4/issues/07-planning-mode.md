# 07 — Planning mode: companion-driven drafting

## Objective
A planning conversation with the companion that produces a draft spec package in `huddle-data`, checked against an embedded Milestone Spec Standard.

## Context
Planning authors the spec package; issue 08 publishes it. The companion is read-only. See README §1, §4.

## Requirements
- A planning conversation with the **read-only** companion that drafts a README + per-issue specs to an embedded **Milestone Spec Standard**.
- A **checker** validates the draft: required sections present; each issue carries what + how-we-know-done; **flag process language** in issue text; the dependency graph is acyclic.
- Drafts **persist** in `huddle-data` across sessions (batched commits) and are resumable.
- The companion performs **no GitHub writes**.

## Out of scope
Publishing (08).

## Success criteria
- A planning session yields a well-formed draft package in `huddle-data`.
- The checker flags process language, missing sections and dependency cycles.
- A draft resumes across sessions.
- The companion makes no writes (verify `--setting-sources user`).

## Evidence required
Tests over good/bad draft fixtures; a sandbox planning session transcript producing a draft.

## Dependencies
02
