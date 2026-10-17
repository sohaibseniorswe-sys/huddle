# 04 — Resume interrupted runs & reconcile drift

## Objective
Resume a build after a hard stop and reconcile the registry against GitHub; handle stale paths and abandoned plans.

## Context
The most common real startup case is resuming a run cut off by a usage limit, hibernate or crash. See README §3–4.

## Requirements
- **Resume** a building project at the correct turn after a hard stop; reconcile the registry against the control issue first — the control issue wins.
- **Drift**: control closed by hand or issues reopened → during a run, reopen + continue; at startup, ask. Honor the existing done-rules.
- **Stale / moved / deleted directory** → prompt to relocate or drop.
- **Abandoned planning draft** persists; is resumable; is prunable.

## Out of scope
The build loop itself (exists in v3).

## Success criteria
- A mid-state building fixture resumes at the right agent/turn.
- Each drift case is handled as specified.
- A stale path prompts relocate/drop; an abandoned plan reappears and resumes.

## Evidence required
Fixture tests for each case; one sandbox resume run (isolated root, huddle-sandbox) for the live-loop path.

## Dependencies
03
