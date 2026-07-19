# Connect CLI

## User Story

As a device-side developer, I want `gizclaw connect` commands to work against the setup server with saved CLI contexts.

## Covered Behaviors

- Context-backed `connect ping` works across repeated, concurrent, reconnect, and missing-server cases.
- `run-status` and a bounded small-payload `test-speed` invocation succeed against the Docker stack.
- `say` owns process-level argument validation; provider audio behavior is exercised by the Go chat and gameplay live suites.
- Contact, friend, and friend-group command trees own real-binary help and argument contracts; stateful social behavior is exercised once by the ordered Go social suite.
- Firmware and gameplay command trees have real CLI round-trip stories for their maintained state-changing paths.
- Public server reads and HTTP login paths work from real CLI contexts.
- Peer metadata preparation remains observable through connect/admin CLI flows.
- Invalid or missing contexts fail with user-facing errors.
