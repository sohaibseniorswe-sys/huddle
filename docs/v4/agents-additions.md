# AGENTS.md — lines to merge for the v4 build

Merge these into the repo-root `AGENTS.md` (create it if absent). Do not overwrite existing content.

- **Working branch:** `v4`. Branch each issue off `v4`; open PRs into `v4` (never `main`, never `v3`).
- **Builds:** build test binaries to `./bin/huddle-v4`. **Never** run `go install` — it overwrites the frozen prod binary at `~/go/bin/huddle`.
- **Real / end-to-end runs:** use the **huddle-sandbox** repo with an **isolated state root** (`HUDDLE_HOME=<tmpdir>`). Never launch a huddle instance against this project's live v4 control issue or the default `~/.huddle` state root.
- **Progress:** append to `docs/v4/progress.md` — start/finish time, what was built, deviations, anything that needs the human.
- **Spec is the source of truth:** read `docs/v4/README.md` first, then the issue's spec in `docs/v4/issues/`.
