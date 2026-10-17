# Terminal selection verification

2026-10-17 00:02 EDT. Controlled PTY replay of the opt-in
`TestSelectionTerminalDemo` fixture at 100 × 34. It uses the production Bubble Tea
model/renderer, synthetic huddle-sandbox output, an isolated `HUDDLE_HOME`, no
build loop, no live agents and no GitHub writes.

Actions and observations:

1. Press/drag from raw-output row 00 beyond the bottom edge. Hold for 400 ms.
   The viewport scrolls while the original anchor stays on row 00.
2. Release, then Ctrl+C. The real renderer emits OSC 52 with 18 selected rows.
   Base64-decoded clipboard text starts with `sandbox selectable output row 00`.
3. Escape clears the selection. A backward drag beyond the upper edge scrolls
   upward. Release and Ctrl+C emit a second payload with 12 selected rows.
4. The terminal transcript includes the selection background color
   `48;2;58;52;86` (the existing SelectBg token). Clipboard payloads contain plain
   text and no display styling.
5. Escape, then Ctrl+C exits cleanly; the terminal fixture reports PASS.

Unit tests additionally cover Unicode cell slicing, forward/backward selection,
keyboard scrolling with selection retained, companion output selection, resize
invalidation, and copying without interrupting the build.

The clipboard transport was verified through the actual terminal renderer;
a desktop paste target is unavailable in this headless environment. Clipboard
integration requires a terminal that permits OSC 52 (including any SSH/tmux
clipboard policy). Ctrl+C is the in-app copy key with an active selection;
Escape clears it before Ctrl+C can quit. Ctrl+Shift+C remains a terminal binding
in terminals that intercept it; if forwarded as Ctrl+C, it copies in-app too.

For an interactive replay, build the test fixture at the canonical test-binary
path, then restore the normal binary afterward:

```sh
go test -c -o ./bin/huddle-v4 ./internal/app
HUDDLE_SELECTION_DEMO=1 ./bin/huddle-v4 -test.run='^TestSelectionTerminalDemo$' -test.v
go build -o ./bin/huddle-v4 ./cmd/huddle
```
