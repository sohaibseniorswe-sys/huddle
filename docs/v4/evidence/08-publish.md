# Real sandbox publish

2026-10-16 23:51 EDT. Isolated state root; approved fixture package; dedicated working branch on `sohaibseniorswe-sys/huddle-sandbox`. The evidence branch is retained for review. Cleanup closed fixture issues and milestone.

Command: `HUDDLE_V4_SANDBOX=1 go test ./internal/integration -run '^TestSandboxPublish$' -v`

```text
=== RUN   TestSandboxPublish
    publish_test.go:157: REAL sandbox publish: milestone https://github.com/sohaibseniorswe-sys/huddle-sandbox/milestone/15; control https://github.com/sohaibseniorswe-sys/huddle-sandbox/issues/53; implementation issue map[01-response:52]; superseded #51 closed and linked; labels ready; exact specs at https://github.com/sohaibseniorswe-sys/huddle-sandbox/tree/v4-publish-evidence-20261017-035112/docs/v4-publish-20261017-035112; second active publish refused without branch changes.
--- PASS: TestSandboxPublish (19.61s)
PASS
ok  	github.com/sohaibseniorswe-sys/huddle/internal/integration	19.616s
```
