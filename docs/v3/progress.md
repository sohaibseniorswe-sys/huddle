# Huddle v3 progress

One entry per issue: when it was built, what was built, where it differs from the spec, and anything that needs the owner.

## v3.2 Companion and decisions

- **Started:** 2026-10-14 00:56 EDT (first timestamp recorded; reading the specs began a few minutes earlier)
- **Finished:** 2026-10-14 01:21 EDT (PR #8 merged into `v3`)
- **Branch / PR:** `v3.2-companion` → `v3`, https://github.com/sohaibseniorswe-sys/huddle/pull/8

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

## v3.3 Agent choice and config

- **Started:** 2026-10-14 01:57 EDT, in a session that ended about two minutes in, after reading the code and creating the branch (nothing written). Restarted 15:12 EDT; the work below was done from then on.
- **Finished:** 2026-10-14 16:00 EDT (PR #9 merged into `v3`)
- **Branch / PR:** `v3.3-agent-config` → `v3`, https://github.com/sohaibseniorswe-sys/huddle/pull/9

### What was built

- **`internal/config`** (new): the per-project file `~/.huddle/projects/<owner>_<repo>.yaml` with `gopkg.in/yaml.v3`. `Load(home, repo)` returns the defaults when there is no file (Dev = Codex, PM = Claude Code, companion same as the PM, CLI defaults, autonomy 8) and starts from them when there is one, so a missing key keeps its default; an unknown key (a typo) is a parse error. `Save` writes a temp file and renames it, with a fixed comment header. `Validate` reports every problem at once, naming its key: unknown CLI, effort not valid for that CLI, autonomy outside 0–10. `Team()` turns the file into what plays each role; `AutonomyName` names the dial's bands for `/settings`.
- **`internal/agents`** (new): the CLI table (`Codex()`, `Claude()`, `All()`, `ByID`) with known models, efforts and the wizard's suggested effort; `Choice` (CLI, model, effort) and `Team`; `Command(role, choice, sessionID, lastMessage)` for every role × CLI, fresh and resumed; `Read` (session id, final message, failure, model used) per CLI; `Usage` (usage limits per CLI); `Validate(ctx, cli, model, effort)`. Session ids, final-message extraction and usage-limit detection moved here from `internal/runner` unchanged, with their tests and recorded output.
- **Roles are independent of the CLI.** The loop gets a `Team` and names roles with their tool ("Dev (Claude Code)"); the prompts say "YOUR ROLE: DEV" / "YOUR ROLE: PM" with "You are the Dev agent (Claude Code)" (`{{agent}}`), and no longer name Codex in the shared or PM text (MVP_PLAN.md section 9 updated to match, and a note added to section 6); usage limits are read with the role's CLI; `CLIAgents` picks the command, Activity parser and output reader by CLI. The status bar, `/status` and the companion's context name the real tool; usage is shown per CLI, once per CLI.
- **The companion** runs on its own CLI, model and effort (or the PM's), on either CLI: Codex with `sandbox_mode="read-only"`, Claude Code as before. A resumed run that reports no session id starts fresh once, for both CLIs. The PM's last output for the context block is read from a Codex PM's `.last` file too.
- **`/settings`** (screen 09): config file path (or "not written yet"), repo, folder, control issue, each role's CLI with version and login, model and effort (plus the model its last run really used, from Codex's header or Claude Code's envelope), autonomy, logs and session ids, and the read-only footer. Esc returns to Activity.
- **Startup:** a settings check (prints what plays each role), then only the CLIs in use are checked for login; their versions are recorded for `/settings`.
- **macOS:** CI runs on `ubuntu-latest` and `macos-latest`. The runner tests' "is this process alive" check read `/proc`, which on macOS made it always pass; it now uses `ps -o stat=`.
- **v3.2 follow-ups:**
  - A waiting card is the first Tab stop after the text field (text field → card → Activity → Companion), the half-typed text stays, Tab on the card moves on, Esc goes back, `/` on the card opens the menu as in the panes. `/decision` still works from the empty text field. The card's hint says "tab or /decision to answer"; README focus section updated.
  - The companion's instructions say to reply in plain text without Markdown.
  - Claude Code has `--setting-sources <sources>` ("Comma-separated list of setting sources to load (user, project, local)"). The Claude Code companion now runs with `--setting-sources user`, so a target repo's `.claude/settings.json` and `.claude/settings.local.json` never apply to it. Recorded in the README with the check below.

### How the success criteria were checked

- **Config tests** (`internal/config`): defaults, a full file, missing keys keep defaults, unknown keys and bad YAML rejected, empty file, save and reload (header kept, no temp file left), invalid CLI, invalid effort per CLI (a Codex-only effort on Claude Code and the reverse), the companion checked only when it has its own settings, autonomy out of range, every problem reported at once.
- **Command building** (`TestCommand`): Dev, PM and companion on Codex and on Claude Code, without and with model and effort, model only, effort only, and resumed sessions for every role and CLI; plus the companion never naming a writing tool or mode on either CLI.
- **Validate** (`TestValidate`, fake CLIs as shell scripts that replay real output): success on both CLIs (and the flags passed), unknown model, bad effort (Codex's API error, and Claude Code's exit-0 warning), not signed in on both, a wrong answer, an effort not in the CLI's list, a timeout (the fake CLI is killed), a missing binary. `TestSmokeValidate` ran against the real CLIs: defaults and a known model at effort `low` passed, `no-such-model-x` was rejected with each CLI's own message.
- **Roles swapped, sandbox milestone "Sandbox M4"** (#22 Truncate, #23 IsBlank, control #24), settings `dev: claude`, `pm: codex`, `huddle --plain`: Claude Code as the Dev merged PRs #25 and #27 and set `state:pm-ready`; Codex as the PM checked the milestone, ran the tests and set `state:done`. About 5 minutes (15:31–15:36). Also covered by `TestSwappedRoles` (loop), `TestCLIAgentsSwappedRoles` and `TestSettingsFileChoosesTheAgents` (main).
- **Non-default model and effort for each role, "Sandbox M5"** (#26 SnakeCase, control #28), on the run screen in tmux: Dev `codex -m gpt-6.1-sol -c model_reasoning_effort="medium"` (the header confirms `reasoning effort: medium`), PM `claude --model sonnet --effort low`, companion (its own) `claude ... --setting-sources user --model haiku --effort low`. The logs' first lines show the flags. I asked the companion what the Dev was doing while Codex worked; it answered. `state:done` after about 4 minutes. `/settings` then showed each role's model and effort and `claude-sonnet-5-5` / `claude-haiku-5-5` as the models the last runs used.
- **/settings golden** (`09-settings`) matches screen 09, with a model and effort line under each agent. `TestViewSwappedRoles` checks the badge and usage with the roles swapped.
- **CI on Linux and macOS:** gofmt, go vet and go test passed on `ubuntu-latest` and `macos-15-intel` (run 38627939008). Nothing in Huddle's own code needed changing for macOS.
- **Companion cannot be given write access by the repo:** with a `.claude/settings.local.json` allowing `Bash(touch:*)` in the sandbox, the companion command without `--setting-sources user` created the file; with it, the call was denied. `TestSmokeCompanionIsReadOnly` (both CLIs, with allow rules for writing and committing planted in the clone) passed: HEAD, `git status` and the files were unchanged.
- **Tab cycle:** `TestTabReachesTheCardFirst`, `TestDecisionCommandWithAnEmptyTextField`, and the existing card tests (which press Tab, then type `/decision`).

### Deviations from the spec

- **`Load(home, repo)`** takes the home folder (and returns whether the file exists) instead of `Load(repo)`, so tests never touch the real home. `Save(home, c)` likewise.
- **More efforts for Codex:** `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`, the values Codex's API lists for `gpt-6.1-sol`; the spec lists five. `max` worked in a real run.
- **Fable added** to Claude Code's known models (`fable`, Fable 5.1): `claude --help` names it as an alias.
- **`DefaultEffort`** is the effort the wizard suggests (`medium` for both); an empty effort in the file still means the CLI's own default.
- **Validate's test run** gets no tools (Claude Code `--tools ""`, Codex read-only with `--skip-git-repo-check`) in an empty temp folder. It also reports a usage limit and a wrong answer. Because Claude Code ignores an unknown `--effort` with only a warning, Validate treats that warning as an error.
- **`/settings` shows "Level 8 · Autonomous (used from v3.5)"**, a model/effort line under each agent, all three session ids, and "N runs of this repo" (the logs do not record the milestone, as screen 09's "this milestone" would need).
- **macOS runs on `macos-15-intel`, not `macos-latest`.** Twice the `macos-latest` (arm64) job was never given a runner and was cancelled after 15 minutes ("The job was not acquired by Runner of type hosted even after multiple attempts", "Due to capacity constraints, jobs targeting macOS arm64 runners may experience longer queue times"). The Intel image ran at once. Switch back when arm64 capacity returns.
- **Help line while a card waits** is now "enter send · tab answer the decision · / commands · ctrl+c quit" (screen 10 showed `/decision` and `tab switch pane`).

### Needs you

- **Codex has been running at `reasoning effort: none`.** With `--ignore-user-config` and no effort, Codex's header says `reasoning effort: none`; every Dev turn so far ran that way. The defaults keep it (the spec says empty = CLI default). To change it, set `agents.dev.effort: medium` (or higher) in `~/.huddle/projects/sohaibseniorswe-sys_palantir.yaml`, or pick it in the v3.4 wizard.
- The companion on Claude Code haiku at low effort still used backticks once despite the plain-text instruction. Larger models follow it better; the text is shown as typed either way.
- Claude Code auto-updated from 2.1.299 to 2.1.300 during this session; nothing changed for Huddle.
- I removed the test settings file for the sandbox, so the sandbox runs on the defaults again, and closed control issue #28 (the PM had left it open with `state:done`).
