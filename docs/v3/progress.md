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

## v3.4 Setup wizard

- **Started:** 2026-10-14 16:20 EDT. A usage limit stopped the session at about 16:55; work resumed at 20:01.
- **Finished:** 2026-10-14 20:09 EDT (PR #10 merged into `v3`)
- **Branch / PR:** `v3.4-setup-wizard` → `v3`, https://github.com/sohaibseniorswe-sys/huddle/pull/10

### What was built

- **`internal/wizard`** (new): the nine-step wizard as a Bubble Tea model (`wizard.go` model and choices, `update.go` keys and steps, `view.go` drawing, `jobs.go` background jobs, `run.go` the program). It has the step list on the left (✓ and value for done steps, ▸ for the current one, later steps dimmed) and the current step's card below.
  1. Finds the git repo around the current folder, or asks for one (`~` works).
  2. Reads `origin`, checks it is the target repo, that `gh` is signed in, and that the account has write access (`viewerPermission` ADMIN, MAINTAIN or WRITE).
  3. to 5. CLI, model and effort chips for each role. Models are the agent table's known ones plus **Custom…**, which opens a text field. Changing a CLI resets its model and effort. The companion can be "Same as PM".
  6. Each CLI in use (installed, version, subscription login), no API keys, then one `agents.Validate` test run per role, one at a time, each with a spinner and then ✓ or the CLI's error. On a failed test run, Enter goes back to that role's step. Choices that passed are not tested again.
  7. The autonomy dial, with the Dev and PM rows for the chosen band.
  8. Creates missing labels. Offers starter `AGENTS.md`/`CLAUDE.md` (embedded templates, shown before writing). At autonomy 6 or above, warns about stop-and-wait rules, with line numbers.
  9. A summary. Then it writes the settings file and the chosen starter files, and commits those files if you choose to.

  Keys: ↑/↓, ←/→, Enter, Esc back one step with every choice kept, Ctrl+C quits without saving. Clicking a chip selects it.
- **`internal/ui/setup.go`**: the setup bar, step rows, chips (`ChipAt` for clicks), fields and card, copied from `mockups.go`.
- **`main`**: `huddle setup` (flags may come before or after `setup`) runs the wizard with the current settings and exits after saving. With no settings file, the wizard runs before anything else, and the start then continues with the new settings. `--plain` with no settings file (or `--plain setup`) prints "Run huddle setup first" and exits with status 2. Outside a git repo, the `dir` from the settings is used.
- **`gh`**: `User`, `Permission`, `IssueView` (labels and open/closed; replaces `IssueLabels`), `ListIssues` (with bodies; replaces `ListOpenIssues`), `ReopenIssue`. **`gitx.Commit`** commits only the named files.
- **Follow-up 1:** v3.3's finish time is now 16:00.
- **Follow-up 2, operating principles** (prompts, MVP_PLAN.md section 9 and the golden tests): spec files are the source of truth (shared section); the Dev completes every issue except the control issue and never closes the control issue; the PM's definition of complete (every issue except the control issue closed, plus the README's Definition of Done and each issue's success criteria and evidence), then the final report, `state:done`, and closing the control issue; the PM states decisions, reasons and facts and never tells the Dev how to work.
- **Follow-up 3, definition of done** (as revised during the session):
  - `state:done` means done, whether the control issue is open or closed.
  - Closed without `state:done` and nothing else open in the milestone: done.
  - Closed with open issues, noticed during a run: Huddle reopens it, posts "Huddle reopened this control issue because issues #x, #y are still open" (marker `<!-- huddle:reopened -->`), and continues with the label it has.
  - Found at startup: "Control issue #N is closed but K issues are still open" with 1) Reopen and continue / 2) Leave it and exit.
  - Control issues are found by the `control` label or `<!-- huddle:control -->` in the body. Every "open issues" check leaves control issues out.
  - Tests: `TestDoneByLabel`, `TestDoneByClosingWithNothingOpen`, `TestClosedDuringARunWithWorkOpenIsReopened`, `TestClosedAtFirstReadWithWorkOpen`, `TestOnlyTheControlIssueOpenIsNotDone`, `TestControlStateClosed`, `TestControlOpenWork`, `TestFetchFactsOnlyTheControlIssueOpen`, `TestStartupClosedControlIssueWithWorkOpen` (reopen, leave, `--yes`), `TestStartupClosedControlIssueNothingOpen`, `TestChooseReopen`, `TestFindControlIssue` ("found by marker when the label is missing"), `TestFindUnfinishedControl`.
- **Follow-up 4:** `~/.huddle/projects/sohaibseniorswe-sys_palantir.yaml` holds the defaults with `dir: /home/sweeng/projects/palantir` and `agents.dev.effort: medium`. It was written with `config.Save`, then loaded again and checked with `config.Validate` (passes).

### How the success criteria were checked

- **Fresh clone opens the wizard; the next start skips it.** `TestFirstRunOpensTheWizardThenSkipsIt` (main). By hand: see the sandbox run below. After it, `huddle` in that clone went straight to the startup checks.
- **`huddle setup` pre-selects every current value.** `TestSetupPreselectsCurrentValues` (custom model shown as Custom…, every role, effort and autonomy) and `TestSetupRunsTheWizardWithTheCurrentSettings` (main).
- **Screens 11, 12, 13 at 100 × 34.** Golden tests `11-wizard-pm`, `12-wizard-companion`, `13-wizard-autonomy`.
- **Bad custom model.** `TestBadCustomModelGoesBackToItsStep` runs the real `agents.Validate` against a fake `claude` that answers with Claude Code's recorded rejection. Step 6 shows `Claude Code rejected model "opux": There's an issue with the selected model (opux)...`, and Enter returns to the PM step with "opux" kept.
- **Changing a CLI resets effort.** `TestChangingCLIResetsEffort` (Codex `none` → Claude Code `medium`).
- **AGENTS.md conflicts at 8, not 3.** `TestConflicts` and `TestPrepareWarnsAboutConflictingRules` (sample files, warnings with line numbers on screen at 8, none at 3; never blocks).
- **Esc at every step.** `TestEscGoesBackOneStepKeepingChoices` walks from step 9 back to step 1 and compares the whole configuration after each Esc.
- Also: `TestFolderStep`, `TestRepoStepErrors`, `TestSaveWritesSettingsAndStarterFiles` (nothing written before step 9; starter written; skipped file not written; commit of exactly the new file), `TestCtrlCQuitsWithoutSaving`, `TestClickAChip`, `TestLongFolderFits`, `TestPlainWithoutSettings`, `TestWizardCanceled`.
- **Manual sandbox check, "Sandbox M6"** (#30 Reverse, control #31, created with the `<!-- huddle:control -->` marker as well as the label). I made a fresh clone with no settings file for the sandbox and ran `bin/huddle-v3 --repo sohaibseniorswe-sys/huddle-sandbox` in tmux at 100 × 34. The wizard opened. In it I chose Dev Codex · GPT-6.1 Sol · medium, PM Claude Code · Sonnet 5.5 · low, the companion "Same as PM", and autonomy 8. Step 6 checked both logins and ran a real test of each choice. Step 8 found the labels and both agent files present. Step 9 wrote `~/.huddle/projects/sohaibseniorswe-sys_huddle-sandbox.yaml`. The startup checks then used it, and the run screen started at 20:04. Codex merged PR #32 and closed #30 (`Reverse` was already in `strutil.go` from an earlier run, so it added the Unicode tests the issue asks for). The PM posted the final report, set `state:done` and closed the control issue (the new principle). At 20:06 Huddle showed "state:done: the milestone is finished" with the report. Running `huddle` again in that clone skipped the wizard, printed the settings line, and stopped at the control-issue check, as expected with no open control issue. The wizard's settings file for the sandbox is kept.

### Deviations from the spec

- **The target repo is still `--repo` or palantir.** Step 2 checks that `origin` is that repo. If it is another repo, the error says to run `huddle --repo <that repo> setup`. The wizard does not switch targets by itself.
- **`huddle setup` exits after saving.** A first-run wizard continues into the run.
- **Card width 74, not 70**, so Claude Code's five model chips (Fable 5.1 is in the agent table) fit on one row. **The PM note** ("Either CLI can play either role; Codex as PM saves Claude Code's limits") is added under screen 11's text, as the spec asks. **Step 3 shows the model's label** ("GPT-6.1 Sol") as the other roles do; the mockup shows `gpt-6.1-sol`.
- **An empty model in the settings pre-selects the CLI's first known model**, because there is no "CLI default" chip. Saving then writes that model (for example `opus`) instead of an empty value.
- **The folder text field is always shown**, pre-filled with the detected repo (or the settings' `dir`), so a different folder can be typed even when one is detected.
- **Nothing is written before step 9:** starter files are shown at step 8 and written at step 9, then committed if chosen (not pushed). Labels are created on GitHub at step 8, as the spec says, so Ctrl+C after step 8 leaves them.
- **More conflict phrases:** besides the spec's four, also "do not push", "don't push", "never merge" and "don't merge". CLAUDE.md is scanned too. The starter AGENTS.md is written to cause no warning at any level.
- **The startup choice is a plain prompt** (`Choose [1/2]`, empty = 1) among the startup checks, because those run before the run screen opens. `--yes` picks Reopen. At startup, only the five most recently closed control issues are examined, and only one without `state:done` whose milestone still has open issues is offered.
- **Huddle never creates control issues yet** (that comes in v3.6), so the marker is only defined (`gh.ControlMarker`) and used to find them. The sandbox control issue made for this run has it.
- **Two bugs found in the manual run and fixed:** a long folder path wrapped the setup bar and the folder field (now cut with "…" and scrolled). Step 6's label column was too narrow for "Claude Code".

### Needs you

- Nothing blocking.
- The palantir settings use the CLI defaults except the Dev's effort (`medium`). Run `huddle setup` in `~/projects/palantir` to pick models through the wizard.

## v3.5 Autonomy level

- **Started:** 2026-10-14 20:55 EDT (checked against git on 2026-10-14: the `v3.5-autonomy-level` branch was created at 20:55:20 from `e9ded0d` and the session's first message arrived at 20:55:13; the v3.5 commit is 20:59:37. 20:55 stands. The earlier session ended at 20:54 after v3.4's finish-time commit at 20:09, and no v3.5 work happened before 20:55)
- **Finished:** 2026-10-14 20:59 EDT (PR opened, CI started)
- **Branch / PR:** `v3.5-autonomy-level` → `v3`, https://github.com/sohaibseniorswe-sys/huddle/pull/11

### What was built

- **Operating principles** (`internal/prompts`, MVP_PLAN.md section 9, kept identical by `TestEmbeddedMatchesPlan`):
  - `shared.md` now has the spec's AUTONOMY section, word for word, right after HOW HUDDLE WORKS, with `{{level}}` filled in.
  - Each role section starts with "You are the Dev agent ({{cli}}), working at autonomy level {{level}}." (or the PM). The `{{agent}}` placeholder is renamed `{{cli}}` to match the spec.
  - Handover rules removed from the role sections, so only the table decides. Dev: "Do not stop to ask for review, approval or confirmation" and "Hand over to the PM (state:pm-ready) only when …" are gone. In their place: "Note any assumption you make in the issue or PR" and "When you hand over to the PM (state:pm-ready), put all open questions and blockers in that one comment". PM: "so the human is never disturbed", "Prefer a sensible decision over a question. If a choice can be changed later, make it, record it, and move on" and "Set state:human-required at any other time only when …" are gone. Added "Record every decision you make" and "When you come to the human, set state:human-required and put the exact question in your comment". The end-of-milestone instructions (final report, `state:done`, closing the control issue, `state:human-required` when only human-only issues remain) are unchanged.
  - `prompts.Launch(role, cli, milestone, control, level)`. `loop.Loop.Autonomy` carries the level into every launch prompt.
- **`--autonomy N`** (main): overrides the settings' level for this run only, without changing the file. Out-of-range values exit with status 2 and `huddle: --autonomy: 11 is outside 0–10 (0 is the most careful, 10 the most independent)`. `fs.Visit` tells a given `--autonomy 0` from no flag. The startup checks print `✔ autonomy 8 · Autonomous`, plus a line naming the override when the flag is used. It is applied after the wizard, so `huddle setup --autonomy 3` still shows and saves the file's level. v1 never had `--dev-autonomy`/`--pm-autonomy` flags, so there was nothing to replace.
- **Run screen:** status bar `palantir · Milestone 3 · control #58 · autonomy 8`; `/status` row `Autonomy  8 · Autonomous` (was "not set"); `/settings` drops "(used from v3.5)".
- **Companion:** the context block has an `Autonomy:` line, for example "level 8 of 0-10. The Dev hands over to the PM only for conflicts between issues, requirement changes or real blockers; the PM comes to the human only when the world is on fire: …".
- **README:** new "Autonomy" section with the table, the `--autonomy` flag, startup check 6, and the settings example.

### How the success criteria were checked

- **Golden tests at 0, 3, 8, 10 for both roles and both CLIs:** `TestLaunchGolden`, 16 files `internal/prompts/testdata/<role>-<cli>-<level>.golden` (they replace `dev.golden`/`pm.golden`). Also `TestAutonomySectionMatchesSpec`, which compares the embedded section with the code block in `docs/v3/issues/05-autonomy-level.md` and checks its position.
- **Old handover rules gone:** `TestOldHandoverRulesGone` renders both roles at every golden level and checks that neither spec phrase appears (nor the three other removed sentences). It also checks that the end-of-milestone instructions are still in `pm.md`.
- **`--autonomy` overrides the config; bad values rejected:** `TestAutonomyFlag` (the file says 6; no flag → 6; `--autonomy 3`, `--autonomy=0`, `--autonomy 10` reach both the loop and the run screen; the file still says 6) and `TestAutonomyFlagOutOfRange` (`11`, `-1`, `100`, `eight` → exit 2, nothing else runs, clear message). Checked by hand with `bin/huddle-v3 --autonomy 12` too.
- **Loop passes the level:** `TestSwappedRoles` now checks "working at autonomy level 3" in both prompts.
- **Calibration:** not done, as you asked (see below).
- No sandbox run: every criterion except calibration can be checked by tests.

### Deviations from the spec

- **Calibration skipped** (your instruction): no sandbox milestone created, no runs at levels 3, 8 and 10, no `docs/v3/autonomy-calibration.md`. The "at level 8 the conflict goes to the PM" criterion is therefore unverified, and the PR leaves that box unticked.
- **Wording added to the role sections** where removed sentences had carried non-handover content: "Note any assumption you make in the issue or PR", "When you hand over …, put all open questions and blockers in that one comment", "Record every decision you make", "When you come to the human, set state:human-required and put the exact question in your comment". These say how to hand over, never when. "If something blocks one issue, leave a note on it and carry on with the rest" stays in the Dev section, because the level-8 expectation ("Dev finishes the rest, hands the conflict to the PM") relies on it.
- **`{{agent}}` → `{{cli}}`**, to use the spec's placeholder name.
- The mockup's "Started Milestone 3 · control #58 · autonomy 8" Activity line is not added. The spec does not ask for it, and the loop has no start event yet.

### Needs you

- **Calibration runs** at levels 3, 8 and 10 on the sandbox, when you want them. Until then, nothing shows that the agents follow their rows.

## v3.6 Start modes

- **Started:** 2026-10-14 21:02 EDT
- **Finished:** FINISH (PR opened, CI started)
- **Branch / PR:** `v3.6-start-modes` → `v3`, PRLINK

### What was built

- **Start menu** (`internal/start`, screens 14 and 15): Plan new milestones, Run an existing milestone (only with open milestones), Resume Milestone N (only when an open control issue has `state:dev-ready`/`-working`/`pm-ready`/`pm-working`/`human-required`; the first queued one wins, else the oldest), Settings (the same text as `/settings`), Quit. ↑/↓, Enter, `q`/Esc. The picker lists open milestones (`gh api repos/{owner}/{repo}/milestones?state=open`) with issue counts (control issues not counted) and the control issue or "control issue created for you". After picking, a text field takes an optional first message for the Dev.
- **Run an existing milestone** (`start.Prepare`): creates the control issue if missing (title `<milestone> Control`, label `control`, body with `## First Dev task` / `Prompt: …` and the `<!-- huddle:control -->` marker), or writes the message into an existing one's First Dev task; then `state:dev-ready`.
- **Plan mode** (`internal/app/plan.go`): run screen with the Companion pane open, status bar "Planning", no facts fetched. The companion's instructions now define the `<plan>` format (always included, so a plan can also be agreed during a run; it is then queued after the running milestone). `companion.Parse` extracts the block with its lines kept; `start.ParsePlan` is a line-based parser with clear errors. The plan card has Agree / Show all / Keep talking (`ui.Card.Buttons` is now a slice). Agree runs `start.Apply`, which creates milestone → issues → control issue per milestone, in order, streaming each step to Activity through `Program.Send`.
- **Milestone queue** (`start.Queue`, `~/.huddle/queue/<owner>_<repo>.json`): `app.Run` now runs a driver on the loop goroutine: it runs the first milestone (or waits for an agreed plan), and after `state:done` removes it from the queue and starts the next one (`Queue.Next` sets `state:dev-ready`, skipping queued issues that were closed by hand). Each start sends a `MilestoneMsg`, so the status bar, facts and `/settings` switch over and Activity says "Started M · control #N · autonomy 8". With the queue empty the badge says Done and any key returns to the start menu. `/status` has a Queue row when something is queued.
- **Flags:** `--resume`, `--run "<title>"`, `--plan` (at most one; `--plan`/`--run` not with `--plain`).
- **gh:** `ListMilestones`, `CreateMilestone`, `CreateIssue`, `EditBody` on the client and the fake.
- **README:** Start menu, Plan mode and Milestone queue sections; flags; startup checks 8 and 9.

### How the success criteria were checked

- **Screens 14 and 15 at 100 × 34:** `TestScreenGoldens` (`internal/start/testdata/14-start-menu.golden`, `15-pick-milestone.golden`, plus `15b-first-message`). I rendered `startMenu()` and `pickMilestone()` from `mockups.go` in ASCII and diffed: 15 is identical; 14 differs only in the note "talk, agree, Huddle creates them" (see deviations). Plan mode goldens were added too (`internal/app/testdata/16-plan-mode`, `16b-plan-card`, `16c-plan-show-all`).
- **Plan parser:** `TestParsePlanOneMilestone`, `…SeveralMilestones`, `…MultiLineBody`, `…MissingFields` (11 cases, each returns an error naming the milestone or issue, and no plan); text outside the block ignored in `companion.TestParsePlan`; `TestABrokenPlanCreatesNothing` checks the error is shown in the conversation and nothing reaches GitHub.
- **Agreeing creates exactly the plan, in order, and queues it:** `TestApplyCreatesThePlanInOrder`, `TestAgreeingAPlanCreatesAndQueuesIt` (fake GitHub call log, queue `[3 5]`, Begin called), `TestApplyStopsAtTheFirstError`.
- **Queue starts the next milestone after `state:done` and survives a restart:** `TestQueueRunsTheNextMilestone` (fake runner: M4 done → M5 starts; Huddle stopped mid-M5; a restart resumes M5 from the queue and M6 follows, queue ends empty), `TestDriveInPlanModeWaitsForBegin`, `TestQueueNext`, `TestQueue`.
- **Running an existing milestone without a control issue creates one; the first message is the First Dev task:** `TestPrepareCreatesAControlIssue`, `TestPrepareExistingControlIssue`, `TestStartMenuRunsAMilestone`, `TestRunFlagUsesTheExistingControlIssue`.
- **End to end in the sandbox:** done with `bin/huddle-v3` in tmux at 100 × 34 (PM and companion Claude Code sonnet/medium, Dev Codex gpt-6.1-sol/medium). Start menu → Plan → one message deciding two milestones → plan card "2 milestones, 2 issues" → Agree. Huddle created milestones "Sandbox M7 · Greet" (issue #33, control #34) and "Sandbox M8 · Farewell" (issue #35, control #36). M7 ran 21:19–21:23 to `state:done`, M8 started by itself at 21:23 and reached `state:done` at 21:25; queue file empty; any key returned to the start menu. Nobody touched anything after Agree.
- **Guidelines:** gofmt, `go vet ./...`, `go test ./...` pass.

### Deviations from the spec

- **Branch base:** branched from `v3.5-autonomy-level`, not `v3`, because PR #11 (v3.5) is open and both touch `progress.md`, `README.md`, `main.go` and the status bar. PR #12's diff includes v3.5's two commits until #11 is merged; merge #11 first and #12 shows only v3.6.
- **Screen 14's note** says "talk, agree, Huddle creates them" instead of "the PM creates them", because section 3.5 says Huddle creates them. `mockups.go` changed to match; the PNG was not regenerated.
- **The confirmation prompt (`Continue? [Y/n]`) is now `--plain` only.** The start menu replaces it. `--yes` still reopens a control issue closed too early, and skips the prompt with `--plain`. `--plain` stays v1: one open control issue, no menu, no queue.
- **Several open control issues are now normal** (a queued plan). The run screen never calls `FindControlIssue`; `--plain` still needs exactly one.
- **Companion files across a queue:** one conversation and session for the whole run screen. In plan mode they are numbered 0 (`…_0.jsonl`, `…_0_companion.json`), so the next `--plan` continues the planning conversation. A restart on a queued milestone starts that milestone's own conversation.
- **Plan steps reach Activity** through `cfg.Send` (Program.Send) from the command's goroutine, and the plan-mode wait uses a one-slot channel. Both are the minimum needed for the loop goroutine Bubble Tea already requires.
- **Added:** `/status` Queue row only when something is queued (screen 06's golden is unchanged); help line "any key start menu" after the last milestone; the start menu's Settings page doesn't scroll (it fits at 100 × 34).

### Needs you

- **Merge order:** #11 (v3.5) first, then this PR.
- **Finished milestones stay open on GitHub.** Neither Huddle nor the PM closes them, so the picker lists M7 and M8 (and older sandbox milestones) as open. Decide whether Huddle should close a milestone when its control issue reaches `state:done`, or the picker should hide milestones with a `state:done` control issue.
- The sandbox now has milestones M7/M8, issues #33–#36 (all closed) and Greet/Farewell on `main`.
