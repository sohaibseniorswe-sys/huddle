# Operating v4

V4 uses a managed-project registry, portable Huddle-owned planning/config storage,
and runtime truth from each project's GitHub control issue.

## Build and isolate

```sh
go build -o ./bin/huddle-v4 ./cmd/huddle
./bin/huddle-v4 --version
./bin/huddle-v4 --home /path/to/isolated/state --list
```

`--home` wins over `HUDDLE_HOME`; otherwise the default is `$HOME/.huddle`.
All state lives below that root. Test binaries use `./bin/huddle-v4`; never use
`go install` over the frozen production binary. Real acceptance runs use only
huddle-sandbox and isolated roots. The #21 acceptance exception permits one private
`huddle-sandbox-new-<YYYYMMDD-HHMM>` repository, archived after the run.

The root contains `huddle-data/` (Git registry/config/drafts), local logs,
sessions, queues and OS lock files. Config/registry changes are committed
immediately. Planning conversations/spec packages commit at response checkpoints.
Legacy config migration retains original YAML bytes and backups.

## Open or adopt

Launch without a target to see cached project status. Listing does no GitHub
calls or next-step computation. Open an entry to refresh its control state and
route to planning, published/start, building/resume, or done/plan-next/archive.
`--list` prints the same cached registry and `--project <id>` opens an entry.
An absent checkout prompts for relocation or dropping the entry from the list.

From an existing checkout, use `--adopt` to plan a fresh milestone or
`--adopt --respec` to normalize existing open issues into replacement specs.
The picker also offers adoption of its current directory. Identity follows the
GitHub remote, falling back to the directory when there is no remote. Adoption
checks GitHub and saves planning input; it makes no GitHub writes.

`--new` creates a planning identity without a name, directory or repo. It persists
and resumes. Decide the name, absolute directory, GitHub owner/repo and working
branch with the companion. Its draft includes a `project` object containing
`name`, `dir`, `repo` and `branch`. The chosen name appears in the registry;
saving the proposal creates no directory or repository. `/publish` reviews the
full destination and package before Agree authorizes creation of a **private**
repository. Huddle creates the directory, initializes the remote through gh/API,
and performs a one-time bootstrap clone before publishing the first milestone.
Bootstrap also supplies local `AGENTS.md` and `CLAUDE.md` starters so first Start
passes its instruction-file checks; agents can customize and commit them.
The directory must be new; an existing repository is accepted only as recovery
of this identity's journaled setup and must retain its private ownership marker.

Setup intent, destination and completed publish steps survive interruption.
Reopen the same project id, review and approve again to retry; completed steps
are reused. A partially started setup cannot be redirected to a second repo.
Repair the original destination/error and resume its proposal. Subsequent
milestones omit the creation object and use the existing registry destination.

## Draft, review, publish

Talk to the read-only companion. It returns a README plus per-issue specs in a
structured draft; Huddle saves them under `huddle-data/drafts/<id>/`. The embedded
Milestone Spec Standard checks required nonempty sections, acceptance evidence,
process instructions and dependency cycles. Bad drafts/candidates persist for
repair. Reopening reloads specs and recent conversation, even without CLI
session metadata. The companion's permissions remain read-only; Claude runs
with `--setting-sources user`.

Use `/publish` to review the exact saved content and destination repo/working
branch. Show all displays the complete package. Agree authorizes that revision
only. Publishing rechecks content and the active-control gate before writes,
commits specs to `docs/<slug>/` on the chosen remote branch, ensures state labels,
creates a milestone and dependency-ordered reference issues, closes/links explicit
superseded issues, and creates one clean dev-ready control. Steps are journaled
for retry. A changed package/destination requires fresh approval.

The resulting project is published; publishing does not implicitly start a
build. Reopen and choose Start. After the new-project bootstrap clone, project
checkout Git operations remain agent-owned;
approved publishing writes spec commits through GitHub's Git Data API.

## Build and plan next

Only one build loop runs per state root, including pauses/usage waits and across
processes. Kernel locks release on crash; metadata alone does not establish a
live build. Planning is a separate lane. A companion using the live agent's CLI
gets a usage-contention warning rather than a block.

V4 Dev sessions complete at most one implementation issue. At autonomous levels,
Dev-ready starts another fresh Dev session without requiring PM review. Every
new Dev/PM turn reloads durable control history, specs, Session.md and git history.
Usage retries within one issue can resume its current session. Explicit legacy
`--repo` entry retains v3 session policy.

Use `--project <id> --plan-next` while building or done. A published draft is
archived once; an unfinished N+1 draft resumes. The live registry phase/control
are preserved while planning. Publishing N+1 stays blocked until N is done.
A closed control without done is reconciled against remaining milestone work;
startup asks before reopening, and a live loop reopens automatically. Explicit
state:done retains the existing done-rule precedence.

`--project <id> --prune-draft` removes an abandoned draft without removing its
identity/config. Archive hides a completed project from the picker.

## Copy output

Drag in raw logs or companion output to select. The anchor persists during
wheel/keyboard scrolling; dragging at either edge autoscrolls. Ctrl+C copies
selected text through OSC 52; Ctrl+Y also copies. Escape clears selection, then
Ctrl+C quits normally. Terminals that reserve Ctrl+Shift+C keep that native
binding; forwarded Ctrl+C copies the in-app selection. Shift-drag is the native
selection alternative. Clipboard writes require terminal/SSH/tmux permission.
Selection clears on resize or a different log window/source; a log window holds
up to 1 MiB. Displayed timestamps are copied when selected; styling/padding are
removed.

## Fresh machine

Commit the current Huddle store and make those commits available in its private
remote (store commits are local; push them before moving machines). Then:

```sh
./bin/huddle-v4 --home /new/state --data-remote <existing-private-git-url> --list
```

The clone contains stable IDs, cached status, settings and planning drafts.
Logs and runtime session metadata stay local. Opening a project refreshes GitHub
runtime truth and lets you relocate a machine-specific checkout path.

## Validation and remaining acceptance

Run `go test ./...`, `go vet ./...`, and build the canonical binary. PR CI checks
Linux and macOS; v3 direct-entry, loop, prompt and golden tests remain in the full
suite. Race detection is unavailable in this image without a C compiler.

See [evidence](evidence) for sandbox planning, publish, onboarding, concurrency,
session isolation and terminal selection checks. Real agent planning is explicitly
identified; orchestration fixtures use scripted Dev/PM processes and real GitHub,
process runner and build loop. Final #24 completion still requires the full
from-scratch #21 flow; an adopted-project run is not new-repo creation evidence.
