# 06 — Concurrency: one build loop, lane separation, usage warning

## Objective
Enforce one build loop at a time, keep planning unrestricted, separate the PM and companion lanes, and warn on usage contention.

## Context
See README §4.

## Requirements
- Refuse to start a **second build** while one is building, with a clear message.
- Allow **planning any project** while another builds.
- The **PM** (build agent) and the **companion** (planning agent) are separate lanes; planning Y must not block X's PM.
- **Warn** when the companion CLI equals the live build's agent CLI (usage contention) — a warning, not a hard block.

## Out of scope
Role-swap on usage limit.

## Success criteria
- A second build is refused.
- Planning Y works while X builds (sandbox).
- The warning fires when companion CLI == live build CLI.
- X's PM runs while the companion talks about Y.

## Evidence required
Tests for the refusal and the warning; a sandbox demonstration of concurrent plan + build.

## Dependencies
03
