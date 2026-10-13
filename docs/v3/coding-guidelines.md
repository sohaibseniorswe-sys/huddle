# Coding guidelines for Huddle v3

The owner of this repo is learning Go (coming from Java, C++, Python and Objective-C) and will read and change this code by hand. Write every line so that person can follow it. When two solutions work, choose the one that is easier to read.

## Style: Google's Go Style Guide

Follow the [Google Go Style Guide](https://google.github.io/styleguide/go/) (its *Guide*, *Decisions* and *Best practices* pages). Its priorities, in order: **clarity, simplicity, concision, maintainability, consistency**. Where this file and the guide disagree, this file wins.

- `gofmt` formats everything. `go vet ./...` passes with no warnings.
- Names: short, lowercase package names that are nouns (`activity`, `config`, `wizard`). No `util`, `common`, `helpers` or `misc` packages.
- Exported names read well with their package: `config.Load`, not `config.LoadConfig`.
- Errors are values: return them, wrap them with context (`fmt.Errorf("load config %s: %w", path, err)`), handle them where something can be done about them. Never ignore an error silently; if ignoring is right, write a comment saying why.
- No `panic` for normal failures.

## Comments: generous and explanatory

- Every package has a package comment explaining what it is for and how it fits into Huddle.
- Every exported type, function, method and constant has a doc comment that starts with its name.
- Unexported functions that do anything non-obvious get a comment too.
- Comments explain **why**, not just what. Where Bubble Tea or Go itself does something a Java or Python programmer would not expect, explain it in a sentence. Examples worth explaining: the Update/View message loop, why `Update` returns a new model, value versus pointer receivers, what a `tea.Cmd` is, why a goroutine sends messages with `p.Send`, what `context.Context` cancellation does.
- Long functions get short comments that mark their steps.
- Keep comments true: when you change code, update its comments.

## Keep it simple: no clever Go

Prefer the boring, obvious version.

- **No** reflection, no `unsafe`, no code generation, no `init()` side effects.
- **No** generics unless a function is genuinely needed for several types and the generic version is clearly simpler. Write the concrete version first.
- **No** global mutable state. Pass dependencies in (structs or function parameters) so tests can replace them.
- **No** deep struct embedding or embedding used to "inherit" behaviour. Use named fields.
- **No** channels or goroutines beyond what Bubble Tea and running agent processes require. Bubble Tea already gives you a single-threaded update loop: use it.
- **No** functional-options patterns, builder chains or interfaces with one implementation "for later". Add an interface only where a test needs a fake (GitHub, agent runner, clock).
- Prefer a plain `switch` over a map of functions; prefer a `for` loop over a chain of helpers; prefer early `return`s over nested `if`s.
- Keep functions short enough to read on one screen. Split them by step, not by line count.
- Constants with names instead of magic numbers (`const projectPaneRows = 2`).

## Project structure

Structure the code the way a person would look for it:

- `cmd/huddle/main.go` only parses flags, wires things together and starts the right screen. No business logic.
- One concept per package under `internal/`. See the target layout in [README.md](README.md).
- Bubble Tea models: one model per screen or reusable component, each in its own file (`model.go`, `update.go`, `view.go` when a model grows past ~200 lines).
- Styles (colours, borders, paddings) live in one place, `internal/ui/styles.go`, copied from [screens/mockups.go](screens/mockups.go). Views never invent colours.
- Messages (types sent to `Update`) are declared next to the code that sends them, with a comment saying who sends them and when.

## Tests

- Tests live next to the code (`foo_test.go`). Table-driven tests where there are several cases.
- Fakes for GitHub, the agent runner and the clock, behind small interfaces.
- **View tests:** render a model's `View()` at 100 × 34 with Lip Gloss's colour profile set to ASCII (`lipgloss.SetColorProfile(termenv.Ascii)`), and compare with golden files in `testdata/`. Provide a `-update` flag to rewrite goldens.
- **Update tests:** send key and window-size messages to `Update` and check the resulting model (focus, open panes, selected line). No real terminal needed.
- Real agent CLIs are only used by opt-in smoke tests (`HUDDLE_SMOKE`), and only against the sandbox repo.

## Dependencies

- Standard library first.
- For the terminal UI: [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles) (`textinput`, `viewport`, `spinner`, `key`) and [Lip Gloss](https://github.com/charmbracelet/lipgloss), on their v1 release lines.
- For the config file: `gopkg.in/yaml.v3`.
- Pin exact versions in `go.mod`. Choose versions released at least two weeks ago.
- Add any other dependency only with a sentence in the PR explaining why the standard library is not enough.

## Every pull request

- One issue per branch and PR, as AGENTS.md describes.
- `gofmt`, `go vet ./...`, `go test ./...` pass.
- README.md is updated for anything a user would notice.
- The issue's success criteria are ticked off in the PR description, with a note on how each was checked.
