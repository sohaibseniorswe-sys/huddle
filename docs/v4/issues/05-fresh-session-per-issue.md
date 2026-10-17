# 05 — Fresh agent session per issue

## Objective
Start a fresh agent session per issue instead of reusing one session across the milestone; reload state from durable sources each issue.

## Context
The build loop currently reuses one session per turn, so context compounds across every issue and burns the session budget — the likely cause of the mid-run usage limit. See README §5.

## Requirements
- Each issue's Dev turn (and each PM turn) runs in a **new** agent session; no accumulated transcript across issues.
- State needed for an issue is reloaded from durable sources — the control issue history, `Session.md`, git log — not from in-session memory.
- Handle per-CLI mechanics: Codex and Claude Code start/resume differently; verify a fresh session reloads state rather than silently losing it.

## Out of scope
Role-swap on usage limit (separate backlog).

## Success criteria
- In a multi-issue sandbox run, process args/logs show a fresh session per issue.
- Context does not accumulate across issues.
- Agents still complete issues with the state they need.

## Evidence required
A sandbox run (isolated root, huddle-sandbox) with per-issue session evidence; a before/after context-size note.

## Dependencies
03
