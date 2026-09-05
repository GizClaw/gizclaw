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

## OTA status

`Server.PutOTAStatus` persists device telemetry in a dedicated per-peer OTA KV record using atomic compare-and-mutate, protecting terminal states from concurrent or out-of-order progress. `GetStatus` projects it as `PeerStatus.ota`; ordinary `PutStatus` does not write the OTA record, so control responses cannot overwrite update progress. Stores must support atomic create-if-absent and compare-and-mutate; unsupported stores return an error. See [Telemetry API](/en/developing/api/proto/telemetry#ota-reporting) for fields and ordering.
