# CLI User Story Sets

`tests/gizclaw-e2e/cmd` contains process-level e2e tests for the real `gizclaw` CLI binary built by the Docker e2e test runner.

These tests execute `testdata/bin/gizclaw` through `os/exec`. They should not use typed clients as the primary assertion path, and they should not use `go run`.

## Command Groups

- `root`: top-level help and root dispatch compatibility.
- `gen-key`: key generation CLI behavior.
- `context`: saved context lifecycle commands.
- `serve`: foreground server workspace lifecycle.
- `service`: service-managed server lifecycle guardrails.
- `migrate`: workspace migration command behavior.
- `connect`: device/client-facing connect commands.
- `admin`: admin CLI resource and peer-management commands.
- `play`: Play UI launcher command behavior.

Each command group owns one `USER_STORIES.md` file and focused `_test.go` files.
