# CLI User Story Sets

## Goal

`test/gizclaw-e2e/cmd/` contains process-level CLI integration tests driven by `go test`.

Each story set should verify real user workflows by:

- starting real `gizclaw` subprocesses
- using an isolated virtual `HOME`
- using an isolated `XDG_CONFIG_HOME`
- using a story-owned server workspace and config
- keeping all runtime artifacts inside the test harness sandbox

## Story Set Layout

Each story directory should contain:

- `USER_STORIES.md`
- one or more `*_test.go` files
- optional story-owned config fixtures

The shared process harness lives in:

- `test/gizclaw-e2e/cmd/test_harness.go`

## Case Naming Convention

Use one directory per case:

- `000-ping`
- `001-ping-reconnect`
- `100-context-create-use-list`

Recommended rule:

- `000-099`: connectivity smoke
- `100-199`: context lifecycle
- `200-299`: server workspace lifecycle
- `300-399`: single-client workflows
- `400-499`: multi-client workflows
- `500-599`: admin and peer workflows
- `600-699`: idempotency and repeatability
- `700-799`: failure and recovery

Each case directory should describe:

- what long-running processes are started
- what CLI commands are executed
- whether the scenario is sequential or concurrent
- what user-visible outcome is expected

## Planned Story Sets

### `000-*` Connectivity Smoke

Purpose:
- prove the basic CLI workflow is alive end to end
- keep the first stories small and stable

Cases:
- `000-ping`
  - start one server from CLI
  - create two client contexts
  - run repeated `gizclaw connect ping`
  - run small concurrent ping across two clients
  - status: implemented
- `001-ping-reconnect`
  - start one server
  - create one client context
  - run `ping`
  - stop and restart the server with the same workspace
  - run `ping` again and verify the same context still works
  - status: implemented
- `002-ping-multi-round`
  - start one server
  - create multiple client contexts
  - run several rounds of sequential then concurrent `ping`
  - verify the server remains healthy across repeated rounds
  - status: implemented
- `003-ping-rpc-client-reuse`
  - start one server
  - create one client context
  - connect one long-lived client session from the created context
  - issue repeated RPC `Ping` requests over the same live connection
  - verify repeated requests do not fail with premature EOF
  - status: implemented

### `100-*` Context Lifecycle

Purpose:
- validate the user-facing context management workflow

Cases:
- `100-context-create-use-list`
  - create multiple contexts against the same server
  - switch current context with `gizclaw context use`
  - run `gizclaw context list`
  - verify the active marker and output ordering
  - status: implemented
- `101-context-current-default`
  - create one context
  - use it as current
  - run commands without `--context`
  - verify the current context is used by default
  - status: implemented
- `102-context-duplicate-create`
  - create a context once
  - try to create the same context again
  - verify the command fails with a clear user-facing error
  - status: implemented
- `103-context-isolation-between-story-homes`
  - create contexts in one story sandbox
  - verify another story sandbox starts empty
  - status: implemented
- `104-context-info-show`
  - show current context details
  - show a named context after listing contexts
  - verify `server-info` uses a selected context
  - verify the legacy `ctx` alias is unavailable
  - status: implemented

### `200-*` Server Workspace Lifecycle

Purpose:
- validate CLI server startup against real workspace fixtures

Cases:
- `200-server-config-boot`
  - boot from a story-owned `config.yaml`
  - verify listen address comes from the workspace config
  - verify the process serves requests at that address
  - status: implemented
- `201-server-identity-persistence`
  - start a server workspace
  - record the generated public key
  - restart using the same workspace
  - verify the public key is reused
  - status: implemented
- `202-server-workspace-isolation`
  - start two servers from two separate workspaces
  - verify they generate different identities and isolated data dirs
  - status: implemented
- `203-server-clean-shutdown`
  - start one server
  - stop it via test-controlled signal
  - verify the process exits cleanly and the workspace remains reusable
  - status: implemented

### `300-*` Single-Client Public Flows

Purpose:
- validate one prepared client context and its peer RPC device-facing flows
  without depending on removed `play` CLI subcommands

Cases:
- `300-client-public-read-sequence`
  - prepare one device/client against the server through the harness API
  - read device info through peer RPC
  - verify the context remains usable through `ping`
  - status: implemented
- `301-client-register-then-read`
  - prepare one device/client against the server
  - read back registration info through admin CLI
  - verify user-visible state transitions
  - status: implemented
- `302-client-public-retryable-reads`
  - prepare one device/client against the server through the harness API
  - repeat peer RPC info reads
  - verify `ping` remains stable between reads
  - status: implemented

### `400-*` Multi-Client Coordination

Purpose:
- validate multiple independent client homes/contexts against one server

Cases:
- `400-multi-client-concurrent-ping`
  - create multiple client identities
  - ping the same long-running server concurrently
  - keep the concurrency modest and deterministic
  - status: implemented
- `401-multi-client-sequential-isolation`
  - run mixed commands from different client contexts in sequence
  - verify one client’s actions do not leak into another client’s context state
  - status: implemented
- `402-multi-client-home-isolation`
  - run clients from separate virtual homes
  - verify context files and generated identities stay isolated
  - status: implemented
- `403-multi-client-reconnect-race`
  - reconnect multiple clients against one reused server
  - verify existing contexts continue to work after reconnect waves
  - status: implemented

### `500-*` Admin And Peer Workflows

Purpose:
- validate higher-level CLI user stories after the base harness is stable

Cases:
- `500-admin-context-provision`
  - provision an admin-capable context against a prepared server fixture
  - verify admin CLI commands can connect successfully
  - status: implemented
- `501-admin-list-peers`
  - prepare server state with one or more peers
  - run admin/peer listing commands through CLI
  - verify output shape and target selection
  - status: implemented
- `502-admin-lookup-peer`
  - exercise lookup commands by public key / serial-like identifiers when supported
  - verify a clean not-found path and a successful path
  - status: implemented
- `503-admin-config-flow`
  - run the smallest end-to-end peer config workflow exposed by CLI
  - verify both happy-path and user-visible failure messages
  - status: implemented
- `504-admin-credentials`
  - verify credential list/get through admin CLI
  - status: implemented
- `505-admin-minimax-tenants`
  - verify MiniMax tenant list/get through admin CLI
  - status: implemented
- `506-admin-voices`
  - verify voice list/get and filters through admin CLI
  - status: implemented
- `507-admin-workflows`
  - verify workflow list/get through admin CLI
  - status: implemented
- `508-admin-workspaces`
  - verify workspace list/get through admin CLI
  - status: implemented
- `509-admin-resources`
  - verify declarative resource apply create/update, show missing/existing, and delete through the generic admin API
  - status: implemented
- `510-admin-ai-provider-catalog`
  - verify provider tenant, model, and ACL view resources through admin CLI
  - status: implemented
- `511-admin-firmwares`
  - verify firmware CRUD, upload, release, rollback, device RPC query/download, and resource show through CLI
  - status: implemented

### `600-*` Idempotency And Repeatability

Purpose:
- ensure the CLI behaves well when users repeat commands

Cases:
- `600-repeat-ping`
  - run `ping` many times against the same context and server
  - verify no degradation in user-visible behavior
  - status: implemented
- `601-repeat-context-use`
  - run `gizclaw context use <same>` repeatedly
  - verify stable output and no config corruption
  - status: implemented
- `602-repeat-server-restart`
  - start and stop the same workspace repeatedly
  - verify the workspace remains reusable and identity persists
  - status: implemented
- `603-repeat-command-after-partial-state`
  - prepare one context through the harness API
  - repeat the same preparation request
  - verify repeated preparation updates metadata predictably and the context remains usable
  - status: implemented

### `700-*` Failure And Recovery

Purpose:
- verify user-visible behavior when the environment is wrong or the server is unavailable

Cases:
- `700-missing-context`
  - run a client command without any current context
  - verify the user gets a clear actionable error
  - status: implemented
- `701-wrong-server-public-key`
  - create a context with the wrong server public key
  - verify connection attempts fail clearly
  - status: implemented
- `702-server-unavailable`
  - stop the server while keeping client context files
  - verify commands fail with a clear connection error
  - status: implemented
- `703-invalid-workspace-config`
  - start `gizclaw serve` with a bad or incomplete config fixture
  - verify startup fails with actionable output
  - status: implemented
- `704-recovery-after-restart`
  - fail a client command while the server is down
  - restart the server
  - verify the same context works again
  - status: implemented
- `705-serve-vs-service-managed-workspace`
  - verify `serve` and `serve -f` reject service-managed workspaces
  - status: implemented
- `706-play-ui-only`
  - verify `play` exposes only the UI `--listen` entry
  - verify removed `play` subcommands fail as unknown commands
  - verify `play --listen` prepares the selected context
  - status: implemented

## Ordering Strategy

Recommended implementation order:

1. `000-*` connectivity smoke
2. `100-*` context lifecycle
3. `200-*` server workspace lifecycle
4. `700-*` failure and recovery
5. `400-*` multi-client coordination
6. `600-*` idempotency and repeatability
7. `300-*` single-client public workflows
8. `500-*` admin and peer workflows

## Design Notes

- Early story sets should prefer narrow, reliable workflows over broad coverage.
- Concurrency stories should start with a small number of clients and commands, then expand gradually.
- Each story set should own its fixture config rather than relying on shared mutable global state.
- Story sets should stay readable as user workflows first, harness mechanics second.
- A case should test one coherent user behavior, not an entire product surface.
- If one case grows multiple independent assertions, split it into multiple directories.
