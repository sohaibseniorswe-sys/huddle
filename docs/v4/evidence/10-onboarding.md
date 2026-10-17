# Real sandbox onboarding

2026-10-16 23:55–23:56 EDT. Existing sandbox repo cloned to a temporary directory and adopted into an isolated state root. Evidence branch retained; fixture issues and milestones closed.

Command: `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run '^TestSandboxOnboarding$' -v`

```text
=== RUN   TestSandboxOnboarding
    publish_test.go:241: REAL onboarding: remote adopted into stable id 11e2c2a4388d1628d230fbb91458f232; normalized issue #54; N+1 drafted while PM-working and gated; done-project entry resumed N+1; then published control https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/58.
    publish_test.go:243: REAL sandbox publish: milestone https://github.com/sohaibseniorswe-sys/huddle-sandbox/milestone/16; control https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/56; implementation issue map[01-response:55]; superseded #54 closed and linked; labels ready; exact specs at https://github.com/sohaibseniorswe-sys/huddle-sandbox/tree/v4-publish-evidence-20261017-035548/docs/v4-publish-20261017-035548; second active publish refused without branch changes.
--- PASS: TestSandboxOnboarding (45.45s)
PASS
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	45.457s
```
