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
