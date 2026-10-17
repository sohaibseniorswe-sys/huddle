# Regression result

2026-10-17. `go test ./...`, `go vet ./...`, `gofmt -l .` and
`go build -o ./bin/huddle-v4 ./cmd/huddle` passed. The suite retains the v3 direct
entry, prompt, loop/session and golden-screen checks alongside v4 coverage.
Feature PRs #26–#35 passed Linux and macOS CI.

Correction to the earlier v4.06 Windows note: the **full application** does not
cross-compile for Windows because the pre-existing process runner uses Unix-only
`Setpgid` and `syscall.Kill` (also present at base commit 53bd4cc). This is not a
new v4 regression. The new file-lock package compiles separately for Windows.
No claim of native Windows application support is made. The canonical Linux
binary was rebuilt after that check. Race checks are unavailable without a C
compiler in this image.

```text
ok  	github.com/sohaibseniorswe-sys/huddle/cmd/huddle	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/activity	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/agents	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/app	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/checks	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/commands	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/companion	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/config	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/filelock	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/gh	(cached)
?   	github.com/sohaibseniorswe-sys/huddle/internal/gitx	[no test files]
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/lanes	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/lifecycle	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/loop	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/onboarding	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/planning	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/prompts	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/publish	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/runner	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/session	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/start	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/state	(cached)
ok  	github.com/sohaibseniorswe-sys/huddle/internal/store	(cached)
?   	github.com/sohaibseniorswe-sys/huddle/internal/ui	[no test files]
ok  	github.com/sohaibseniorswe-sys/huddle/internal/wizard	(cached)
```
