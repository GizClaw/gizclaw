# Agent Host

`Implementation file: peer_agent_host.go`

| Documentation | Features included |
| --- | --- |
| `peer_agent_host.go` | Create an Agent Host dedicated to the current Peer, register the persisted Workflow drivers, and inject borrowed Server capabilities. |

This file is only responsible for Host wiring on the Peer connection. Agent instance, input and output, history, toolkit and running life cycle belong to `services/runtime/agenthost`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `newPeerAgentHost` | Create a Peer-scoped Agent Host, install the Peer GenX provider, and register Flowcraft, DashScope Realtime, Doubao Realtime Duplex, Eino, and the other supported Workflow factories. |

The Resolver reads the top-level Workflow `memory` alias and resolves its
`MemoryLayout`, driver, and typed connection from one owner RuntimeProfile
snapshot. Flowcraft and Eino factories consume the same provider-neutral
`memory.Store` contract, while Graph nodes own Recall and Observe mappings.
Workspace ID is `Scope.AppID`; Peer identity and public keys are not substituted
for `Scope.UserID`.

Runtime Registry uses Workspace as the only live Agent identity. Concurrent
streams in the same Workspace share one concurrency-safe Agent. The final
release closes that generation, and reload reconstructs it from the new
Workflow and RuntimeProfile snapshot. Existing per-invocation model and toolkit
resolution does not introduce another Workspace Agent identity.

The new Workflow factories are independent configuration adapters. They do not
require ToolCall or translate Workspace Toolkit policy into provider-native
tools.
