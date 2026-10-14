# Huddle v3 progress

One entry per issue: when it was built, what was built, where it differs from the spec, and anything that needs the owner.

## v3.2 Companion and decisions

- **Started:** 2026-10-14 00:56 EDT (first timestamp recorded; reading the specs began a few minutes earlier)
- **Finished:** 2026-10-14 01:30 EDT (PR into `v3`)
- **Branch / PR:** `v3.2-companion` → `v3`

### What was built

- **`internal/companion`** (new): the companion agent (`Agent.Ask`: one Claude Code print-mode run per message, fresh session with the embedded instructions first, then `--resume`; falls back to a fresh session once if a resumed run prints no JSON envelope), the context block (`Context.String`, trimmed to 15 events, 3 comments, 40 output lines), reply parsing (`Parse`: `<decision>`, `<reason>`, `<action>`, ignoring tags in code fences, last one wins), and the conversation file (`History`, JSON Lines in `~/.huddle/companion/`).
- **`internal/runner`**: `runner.Companion`, the read-only Claude Code command (`--tools` without write tools, `--permission-mode dontAsk`, an `--allowedTools` list of read-only git/gh commands, `--strict-mcp-config`).
- **`internal/session`**: a `companion` role and `session.NewCompanion` for its own session file.
- **`internal/app`**: the Companion pane (spinner while thinking, follows new messages), decision and action cards with the focus rules, Agree / Keep talking / Not now, posting decisions (`<!-- huddle:decision -->`, `state:pm-ready` only from `state:human-required`, then the loop is woken), `/companion`, `/decide`, `/decision`, Tab cycle text field → Activity → Companion, mouse on the Companion pane and card buttons, the Decisions row in `/status`, and the PM's question sent to the companion automatically on `state:human-required`. `/reply` is removed.
- **`internal/ui`**: `Card`, `Chat` and `InputAside` (the greyed text field while a card has the keys).
- **PM operating principles**: "Read any `huddle:decision` comments posted since your last turn and act on them first." (also added to MVP_PLAN.md section 9, which a test compares byte for byte).
- **README**: the companion, its commands and files, the verified read-only flags for Claude Code and Codex.

### How the success criteria were checked

- **Companion never delays the Dev.** `TestSlowCompanionNeverDelaysTheLoop` (a companion that blocks until released; a real `loop.Loop` turn with fakes finishes meanwhile, and the screen keeps taking keys). In the sandbox (Sandbox M3, control #19), I asked the companion a question while Codex worked on #17; Activity kept getting Codex events during the companion's run and the reply arrived about 20 s later.
- **Layout.** Golden tests at 100 × 34: `02-companion`, `03-menu` (¼ Activity, ½ Companion, ¼ menu), `08-log`, `10-decision`, `10b-decision-focused`, plus thinking, action card and no-companion variants.
- **Companion cannot change the repo.** `TestSmokeCompanionIsReadOnly` (opt-in, `HUDDLE_SMOKE`) passed against the sandbox: asked to create, commit and push a file, once with the companion's instructions and once with the bare command; HEAD, `git status` and the file system were unchanged. A manual run of the bare command also asked it to comment on an issue: denied.
- **Context test.** `TestContextNamesTheLogViewRunAndLine`: in the Log view opened from the 22:52 Activity line, the block says `Log view, Codex turn 5 (run 1 of 1), opened at 22:52:07 from the Activity line "..."`, plus state, milestone, events and the running agent's output. `TestContextString` checks the block's exact text.
- **Decision parsing.** `TestParse`: well-formed, missing reason, tags in code fences, two decisions (last wins), no tags, and more.
- **Focus.** `TestCardFocus`, `TestEnterInTheTextFieldSendsEvenWithACard`.
- **Agree / Keep talking.** `TestAgreePostsOneDecisionAndHandsToThePM`, `TestAgreeOutsideHumanRequiredKeepsTheLabel`, `TestKeepTalkingPostsNothing`, `TestFailedPostBringsTheCardBack` (fake GitHub).
- **End to end in the sandbox.** Milestone "Sandbox M3": #17 (Capitalize) and #18 (a LICENSE the maintainer must choose), control issue #19. Codex closed #17 and the PM set `state:human-required`; Huddle opened the Companion pane and the companion explained the options; I answered "Go with MIT", a decision card appeared, `/decision` → Agree posted the `huddle:decision` comment and set `state:pm-ready`; the PM started immediately, Codex merged PR #21 (MIT LICENSE), and the PM set `state:done` about 6 minutes after the start.
- **History survives a restart.** `TestConversationSurvivesARestart`; and in the sandbox, restarting Huddle on #19 showed the whole conversation, and the next message resumed the companion's saved session (`--resume` in its log) and remembered the choice.

### Deviations from the spec

- **No `v3` branch existed.** I created `v3` from `v3.1-tui-shell` (v3.1 was never merged anywhere) and pushed it, so this PR targets `v3` as asked. `v3` therefore already contains v3.1.
- **Added CI.** The repo had no CI. I added `.github/workflows/ci.yml` (gofmt, go vet, go test) so "CI passes" means something; it runs on pull requests and on pushes to `main` and `v3`.
- **When the PM sets `state:human-required`**, Huddle sends the companion a message with the PM's latest comment (marked as sent by Huddle) instead of waiting to prefix the human's next message. The pane then opens with the companion already explaining the question, as screen 10 shows.
- **Typing `/decision` with half-typed text.** A `/` typed in a non-empty text field is just a character, so `/decision` cannot be typed there without losing the text. Typing `/` in a pane (after Tab) now opens the menu there and puts the half-typed text aside; it comes back when the menu closes. So: Tab, `/decision`, Enter; Esc returns with the text intact. A click on the card also works.
- **The decision card shows the reason** (dimmed) when there is room; screen 10 shows no reason line. A long statement drops the reason, and an even longer one is cut with "…".
- **Codex companion not wired.** The companion runs on the PM's CLI, which is always Claude Code until v3.3, so only the Claude Code command is in the code. The Codex read-only flags were verified and are in the README for v3.3.
- **Separate session file** for the companion (`…_companion.json` next to the Dev/PM file), because the companion and the loop save at the same time and `Store.Save` reads, changes and rewrites the whole file.
- **Fast input fix.** Bubble Tea delivers quickly arriving text (a paste without bracketed paste, or tmux `send-keys`) as one key message per word, and the text input treated the words "right", "left" and "up" as arrow keys and dropped them. Multi-character key messages are now marked as pastes. Found while driving the sandbox run through tmux.
- **`commands.Command.Args` removed**: no command takes text since `/reply` is gone.

### Needs you

- Nothing blocking.
- The companion's tool restrictions sit on top of your own Claude Code settings: an allow rule in `~/.claude/settings.json` (or a target repo's `.claude/settings.json`) for a writing shell command would let the companion run it. Yours has none today. Its instructions forbid changes as a second line of defence.
- Replies are shown as plain text, so Markdown (`**bold**`, backticks) appears as typed. Rendering Markdown would need another dependency (Glamour); say if you want it.
