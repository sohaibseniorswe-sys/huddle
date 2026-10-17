# v4 coding guidelines

Inherits the v3 guidelines (`docs/v3/coding-guidelines.md`). v4-specific points:

- **Language/UI:** Go + Bubble Tea; screens via Lip Gloss, consistent with v3.
- **State root:** all config, registry, `huddle-data` clone and logs resolve under a single state root — `HUDDLE_HOME` env var or `--home` flag, default `~/.huddle`. Nothing reads or writes a hard-coded `~/.huddle` path directly.
- **Store:** `huddle-data` is Huddle's own repo. Commit config/registry changes immediately; batch planning-draft commits to checkpoints so a live conversation doesn't spray commits.
- **Builds:** test binaries go to `./bin/huddle-v4`. Never `go install` — it would overwrite the frozen prod binary at `~/go/bin/huddle`.
- **Tests:** prefer table-driven tests against a temp state root (`HUDDLE_HOME=<tmpdir>`) with fixtures for GitHub state (control-issue/milestone/label snapshots). A real run is used only when a Success criterion can't be met otherwise.
- **Real runs:** only on **huddle-sandbox**, with an isolated state root, never against a live control issue. Label real-run evidence as real.
- **Companion stays read-only:** keep `--setting-sources user` so repo settings can't grant it writes; the companion never writes to GitHub.
- **No process language in issue text** the tooling generates or checks — issues carry what to build and how we know it's done only.
