# v3.7 Native Windows (optional)

Make Huddle run natively on Windows (PowerShell or Windows Terminal), not only under WSL. Optional: WSL already works, so do this only when someone needs native Windows. Depends on v3.1–v3.6.

Read [README.md](../README.md) and [coding-guidelines.md](../coding-guidelines.md) first.

## Screens

None new: every screen must look the same in Windows Terminal as on Linux. Use [01-default](../screens/01-default.png) and [10-decision](../screens/10-decision.png) as the visual check.

## What to build

### 1. Stopping an agent and everything it started

The runner kills an agent's whole process group, which is a Unix concept. On Windows, use a **job object** instead:

- Split the runner's process handling into `process_unix.go` (`//go:build !windows`) and `process_windows.go` (`//go:build windows`), each with the same two small functions: start a command in its own group or job, and kill that group or job.
- Windows: create a job object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, assign the agent process to it, and close the job to kill everything (`golang.org/x/sys/windows`).
- Comment both files well: this is exactly the kind of code a newcomer to Go and operating systems needs explained.

### 2. Test fakes that run everywhere

The runner tests use bash scripts as fake agents. Replace them with small Go programs in `testdata/fakeagent/` built once in `TestMain` with `go build`. This works on every OS and makes the fakes easier to read.

### 3. Paths, signals, terminals

- Use `os.UserHomeDir` and `filepath.Join` everywhere (check `~/.huddle` handling).
- Ctrl+C: Bubble Tea delivers it as a key on Windows too; make sure `/quit` behaviour is identical.
- Check the box-drawing characters and colours in Windows Terminal and the legacy console; document what is supported.
- Confirm Codex and Claude Code run natively on Windows with the commands from v3.3, and document any differences (for example, whether Claude Code needs Git Bash).

### 4. CI and docs

- Add `windows-latest` to the CI matrix.
- README: Windows install steps in PowerShell, and a note that WSL remains the recommended setup if the agent CLIs behave better there.

## Success criteria

- [ ] `go test ./...` passes on Linux, macOS and Windows in CI.
- [ ] A runner test proves a child process started by a fake agent is killed on timeout and on `/quit`, on all three systems.
- [ ] Manual check on Windows: the wizard, a Sandbox milestone to `state:done`, the companion, a decision card and `/quit` all work.
- [ ] README documents native Windows setup and any limitations found.
- [ ] Code follows [coding-guidelines.md](../coding-guidelines.md).
