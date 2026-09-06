# Peer Run

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun)

`peerrun` Stores the current running status of Peer and its Agent selection. It has an association between the Peer and the run selection and does not own the Agent definition, Workspace, Workflow, or Agent instance lifecycle.

## Core structure and main function

| Function | Effect |
| --- | --- |
| `Server.GetStatus` / `PutStatus` | Read or update Peer runtime status snapshot. |
| `Server.GetRunAgent` | Read the currently saved Agent selection of the Peer. |
| `Server.SetRunAgent` | Stores the new Agent selection. |
| `Server.ResolveRunAgent` | Parse Peer's currently valid running options. |
| `Server.ActivateRunAgent` | Activates the selection and returns the updated running status. |

`peerrun` only saves and parses the selection; the actual starting, stopping and replacing the Agent runtime is completed by `agenthost.Service`.

`GetDebugMode` / `SetDebugMode` persist device-owned debug permissions, defaulting to `off`.
The setting survives reconnects. HTTP authorization reads it on every request and fails closed on storage errors.

## OTA status

`Server.PutOTAStatus` uses a conditional SQL update of `peer_runs.ota_json` to protect terminal states from concurrent or out-of-order reports. `GetStatus` reads status and OTA in one query; `PutStatus` updates only `status_json`. See [Telemetry API](/en/developing/api/proto/telemetry#ota-reporting) for ordering rules.

## SQL and the local directory

`Server.DB` borrows a centrally managed SQLite or PostgreSQL pool. Call `Initialize` at startup; requests do not execute DDL. Each Server uses its own runtime database.

`peer_runs` stores rows by `public_key`, with separate `pending_workspace`, `active_workspace`, `debug_mode`, and `registered_at` columns. Setting Pending preserves Active. Activation uses a conditional UPDATE so an older activation cannot overwrite a newer selection.

`RememberPeer` records peers registered or connected on this Server. Admin `ListPeers` pages through the local `(registered_at, public_key)` index and reads shared registration details only for that page. It does not enumerate central Redis or represent a global device directory. Runtime-only rows without local registration are excluded.
