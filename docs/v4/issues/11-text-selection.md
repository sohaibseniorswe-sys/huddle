# 11 — Selectable & copyable text in agent panes

## Objective
Make text in agent-output panes selectable and copyable.

## Context
Output comes from the agents into viewports rendered with Lip Gloss, which draws styled cells rather than a native text layer. Independent of the lifecycle work.

## Requirements
- Output panes: text is **selectable and copyable**.
- **Click-drag** selects and scrolls up/down while keeping the selection.
- **Copy** with the standard key (e.g. Ctrl+Shift+C).
- Decide and document the approach: an in-app selection model (track drag → highlight → clipboard) vs letting the terminal's own selection through (mouse-capture off in those panes), and note the trade-off with other mouse features.

## Out of scope
Lifecycle work.

## Success criteria
- Text in agent panes selects across scroll and copies with the standard key.
- The chosen approach and its trade-off are documented.

## Evidence required
Manual verification notes / screen capture; unit tests where feasible.

## Dependencies
—
