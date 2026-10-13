# Huddle Version 3

Huddle v3 turns the working v1 loop into an application you set up and drive from one terminal screen. This folder is the specification for the milestone **Huddle Version 3**. Every issue in that milestone points here.

Read this file and [coding-guidelines.md](coding-guidelines.md) before working on any issue. Each issue has its own spec in [issues/](issues/).

## Contents

| Issue | Spec | Depends on |
| --- | --- | --- |
| v3.1 TUI shell | [issues/01-tui-shell.md](issues/01-tui-shell.md) | — |
| v3.2 Companion and decisions | [issues/02-companion-and-decisions.md](issues/02-companion-and-decisions.md) | v3.1 |
| v3.3 Agent choice and config | [issues/03-agent-choice-and-config.md](issues/03-agent-choice-and-config.md) | v3.1 |
| v3.4 Setup wizard | [issues/04-setup-wizard.md](issues/04-setup-wizard.md) | v3.3 |
| v3.5 Autonomy level | [issues/05-autonomy-level.md](issues/05-autonomy-level.md) | v3.3 |
| v3.6 Start modes | [issues/06-start-modes.md](issues/06-start-modes.md) | v3.2, v3.4 |
| v3.7 Native Windows (optional) | [issues/07-native-windows.md](issues/07-native-windows.md) | v3.1–v3.6 |

Build them in this order. Each issue must leave `main` working: `go build ./...`, `go vet ./...` and `go test ./...` pass, and `huddle` still runs a milestone.

## The philosophy (unchanged from v1)

Design to delivery in one go, with no human involvement unless the world is on fire.

- **The Dev agent** (Codex or Claude Code) works through the whole milestone.
- **The PM agent** (Codex or Claude Code) decides when the Dev hands over, and turns the human's decisions into instructions for the Dev.
- **The human never talks to the PM or the Dev.** The human talks to the **companion**, a third agent that knows everything about the run but never changes anything.
- **Only agreed decisions reach the control issue.** The companion drafts a decision; the human agrees on a card; Huddle posts it and hands it to the PM.
- **Huddle adds no limits and no restrictions** to the working agents. Agents have full permissions; the repo is private and everything can be reverted.

## Screens

Every screen below was rendered by real Lip Gloss code: [screens/mockups.go](screens/mockups.go). They show exactly what a Bubble Tea `View()` can produce at 100 × 34. Use them as the visual reference, and reuse the styles and colours from `mockups.go`.

| Screen | Shows |
| --- | --- |
| [01-default](screens/01-default.png) | Activity takes the whole middle |
| [02-companion](screens/02-companion.png) | Companion open: half of the middle |
| [03-menu](screens/03-menu.png) | All three open: ¼ Activity, ½ Companion, ¼ menu |
| [04-menu-filter](screens/04-menu-filter.png) | `/s` filters the menu |
| [05-paused](screens/05-paused.png) | Paused; menu is context-aware |
| [06-status](screens/06-status.png) | `/status` in the Activity slot |
| [07-activity-select](screens/07-activity-select.png) | Selecting an Activity line |
| [08-log](screens/08-log.png) | Raw log opened at that line; companion knows what you see |
| [09-settings](screens/09-settings.png) | `/settings`, read-only |
| [10-decision](screens/10-decision.png) | A decision waits; typing still goes to the text field |
| [10b-decision-focused](screens/10b-decision-focused.png) | After `/decision`: focus on the card |
| [11-wizard-pm](screens/11-wizard-pm.png) | Wizard: PM agent CLI, model, effort |
| [12-wizard-companion](screens/12-wizard-companion.png) | Wizard: companion model and effort |
| [13-wizard-autonomy](screens/13-wizard-autonomy.png) | Wizard: autonomy dial |
| [14-start-menu](screens/14-start-menu.png) | Start menu |
| [15-pick-milestone](screens/15-pick-milestone.png) | Running an existing milestone |

To regenerate the PNGs after changing `mockups.go`: `go run docs/v3/screens/mockups.go` (writes `.ans` files), then `python3 docs/v3/screens/render.py` (needs Pillow, fontTools and the DejaVu fonts). The file has a `//go:build ignore` line, so `go build ./...` skips it.

## Screen layout rules

The run screen is a fixed vertical stack. Rows, top to bottom:

| Area | Height | Rule |
| --- | --- | --- |
| Project pane | 2 rows, fixed | Status bar (project, milestone, control issue, autonomy, state badge) and info line (issues, PRs, CI, usage, elapsed) |
| Activity slot | flexible | Takes all remaining rows when nothing else is open |
| Companion | flexible | When open, takes half of the flexible rows |
| Slot (menu or card) | flexible | The `/` menu, a decision card or an action card |
| Text field | 3 rows, fixed | Always visible, always in the same place |
| Help line | 1 row, fixed | Keys that work right now |

Splitting the flexible rows (`flex = height − 6`):

| Open | Activity | Companion | Slot |
| --- | --- | --- | --- |
| Activity only | all | — | — |
| Activity + Companion | ½ | ½ | — |
| Activity + Slot | ¾ | — | ¼ |
| Activity + Companion + Slot | ¼ | ½ | ¼ |

Integer division; Companion gets the remainder. The menu scrolls inside its slot ("↓ 3 more") instead of growing. Minimum terminal size is 80 × 24; below that, Huddle shows only "Terminal too small: 80 × 24 needed".

**The Activity slot has views.** Activity list, line selection, `/status`, `/log` and `/settings` all render inside the Activity slot. Switching view never changes the slot's size or position. Esc returns to the Activity list.

## Focus

Exactly one element has keyboard focus. It decides where keys go.

| Focus | Default? | Keys it receives |
| --- | --- | --- |
| Text field | yes | typing, ←/→ (cursor), Enter (send), `/` (opens menu) |
| `/` menu | while open | ↑/↓, Tab (complete), Enter (run), Esc (close) |
| Activity | via Tab or click | PgUp/PgDn, ↑/↓ (select line), Enter (open raw log there), Esc |
| Companion | via Tab or click | PgUp/PgDn, Esc |
| Decision or action card | via `/decision` or click | ←/→ (choose), Enter (confirm), Esc (back to text field) |

Rules:

- Keys go **only** to the focused element. A card on screen never steals arrows or Enter from the text field.
- Leaving the text field keeps its half-typed text.
- Tab cycles between the text field and the visible panes. The focused pane has a purple border and "◆ focused".
- Mouse (Bubble Tea mouse support): the wheel scrolls the pane under the pointer; a click focuses a pane, selects an Activity line or presses a card button. Mouse mode stops normal terminal text selection; document that Shift + drag selects text in most terminals.
- Ctrl+C always works: it behaves like `/quit`.

## Slash commands

Typing `/` in the text field switches its prefix from `companion ›` to `command ›` and opens the menu. The list filters as you type (`/s` → `/status`, `/settings`). Commands appear only when they make sense:

| Command | Shown when | Does |
| --- | --- | --- |
| `/status` | always | Shows facts in the Activity slot: issues, PRs, CI, turn, decisions, autonomy, usage, elapsed. No agent is started. |
| `/companion` | always | Opens or closes the Companion pane. Shown lit (●) while open. Closing hides it; the conversation is kept. |
| `/decide` | a companion conversation exists | Asks the companion to draft a decision now |
| `/decision` | a decision, action or plan card waits | Moves focus to the card |
| `/pause` | an agent is running or about to run | The current turn finishes normally; no new turn starts. Badge: "Paused · PM next". |
| `/resume` | paused | Starts the next turn |
| `/log` | always | Raw output of every agent run in the Activity slot, newest run first; `[` `]` switch runs |
| `/settings` | always | Read-only settings in the Activity slot |
| `/quit` | always | Stops the running agent immediately, puts its ready label back, exits. The next start resumes that agent's session. |

## Verified facts about the agent CLIs

Checked against current documentation; the implementer must re-check each against `--help` of the installed versions and record versions in the main README.

| | Codex CLI | Claude Code |
| --- | --- | --- |
| Model | `-m <model>` (or `-c model="<model>"`) | `--model <alias or full name>`; aliases `opus`, `sonnet`, `haiku` |
| Effort | `-c model_reasoning_effort="<level>"`; `none`, `low`, `medium`, `high`, `xhigh` | `--effort <level>`; `low`, `medium`, `high`, `xhigh`, `max` |
| Resume | `codex exec resume <SESSION_ID>` | `--resume <session id>` |
| List models | **No command found.** Only the interactive `/model` picker. | **No command found.** Only the interactive `/model` picker. |

Because neither CLI can list its models, Huddle keeps a small table of known model names per CLI, offers **Custom…** to type any other name, and validates every chosen model and effort with a tiny test run during setup (see v3.3 and v3.4).

## Target project layout

Keep the existing v1 packages. Add new ones like this (adjust names only if they collide with existing ones):

```
cmd/huddle/          main.go: flags, chooses wizard, start menu or run screen
internal/
  app/               Bubble Tea root model of the run screen: layout, focus, routing   (v3.1)
  ui/                Reusable view pieces and styles: panes, bars, menu, cards          (v3.1)
  activity/          Activity events from agent output, with log positions             (v3.1)
  commands/          Slash command list, filtering, visibility                          (v3.1)
  companion/         Companion agent, context building, decision and action parsing     (v3.2)
  config/            Per-project config file                                            (v3.3)
  agents/            Agent table: CLIs, models, efforts, commands, validation           (v3.3)
  wizard/            Setup wizard                                                       (v3.4)
  start/             Start menu, plan mode, milestone queue                             (v3.6)
  loop/ runner/ gh/ checks/ session/ prompts/ ...   existing v1 packages
docs/v3/             this specification
```
