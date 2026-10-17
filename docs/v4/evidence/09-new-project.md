# v4.09 — real new-project acceptance

Run: 2026-10-17 04:28–04:29 UTC. **PASS**.
PM authorization: control #25, decision 6497984 and issue 09's PM decision.
Exactly one new private repo was created:
[sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427).
It is archived, with history retained. No live Huddle control or default state root was used.

```sh
go build -o ./bin/huddle-v4 ./cmd/huddle
HUDDLE_V4_NEW_REPO=sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427 \
HUDDLE_V4_NEW_HOME=/tmp/huddle-v4-new-20261017-0427 \
go test ./internal/integration -run '^TestNewProjectLifecycle$' -count=1 -v
```

This command creates a repository: do not rerun it with a new name. Its explicit
root is durable for investigation; ordinary tests skip this opt-in acceptance.
The historical log is `/tmp/huddle-v4-new-20261017-0427.log` on the acceptance host.

The initial registry identity `new-project-acceptance` had no name, dir or repo
and phase `planning`. A complete saved draft proposed name `New project acceptance`,
directory `/tmp/huddle-v4-new-20261017-0427/workspace`, the private repo above,
and branch `v4-acceptance`. Before approval, the directory did not exist.
A second Store/Workspace loaded the same committed identity and complete package,
then reviewed the same destination. Planning resumed without recreating identity
or requiring project attributes at initial creation.

Scripted approval bound the exact draft and destination. Real publishing created
the directory, private initialized GitHub repository and branch, performed the
setup-only clone, then published:

- [Milestone 1](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/milestone/1)
- [Implementation #1](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/1)
- [Control #2](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/issues/2), initially dev-ready
- [Exact specs](https://github.com/sohaibseniorswe-sys/huddle-sandbox-new-20261017-0427/tree/v4-acceptance/docs/sandbox-lifecycle-fixture)

Registry id stayed unchanged; phase became `published`. Settings retained the
same identity and acquired the real repo/path. Reopening the canonical binary
started the published milestone from the new checkout. Dev pulled the API-written
specs, verified the fixture, and closed #1. PM completed #2. These workers were
**scripted Dev/PM processes**; GitHub, clone, binary, process runner, build loop,
phase refresh and store were real. This run does not claim new live-model work.
The separate real read-only Claude planning evidence remains in `07-planning.md`.

The same run continued through done → plan-next and fresh-machine listing;
see `12-new-project-lifecycle.md` for those acceptance observations.

Automated success/failure coverage additionally verifies: no creation without
approval; absolute-path/repo validation; review displays PRIVATE/name/path/branch;
existing directories and foreign/public repositories refused; permissions errors
not treated as absence; directory persisted across a simulated creation failure;
second-session retry keeps the same approval revision; clone retry and completed
publish do not duplicate work; partial setup cannot redirect to another repo.
After this acceptance run, chosen names were also cached during draft saving so
the picker labels repo-less projects before approval; this local-only refinement
is covered by the ordinary tests.
