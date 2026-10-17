# 10 — Onboarding flows: adopt, plan-next, plan-while-building

## Objective
Support adopting existing projects, planning the next milestone, and planning while building.

## Context
Cases 1a/1b (adopt), 3 (plan next), 4 (plan while building). See README §2–4.

## Requirements
- **Adopt (1a/1b)**: identify a repo by remote/dir; check GitHub state; then either re-spec open issues through planning and publish (1a — closing + linking superseded issues), or plan a fresh milestone and publish (1b). Drafts must pass the Spec-Standard check before publish.
- **Plan-next (3)**: from a done or building Huddle project, start a planning session for the next milestone.
- **Plan-while-building (4)**: draft N+1 while N builds; publishing N+1's control issue is blocked until N is done (issue 08 gating).

## Out of scope
New-from-scratch (09).

## Success criteria
- Adoption normalizes issues and publishes (sandbox).
- Plan-next works from a done project.
- N+1 drafts fully while N builds and is blocked from publishing until N is done.

## Evidence required
Sandbox demonstrations of each entry point.

## Dependencies
07, 08
