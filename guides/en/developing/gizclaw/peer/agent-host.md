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

The Server resolves `agent_host.flowcraft.memory_store` and
`agent_host.eino.memory_store` once. The Peer AgentHost borrows those
provider-neutral `memory.Store` capabilities; it does not own or close them.
Flowcraft and Eino bind the Workspace ID only to `Scope.AppID`. Peer identity
and public keys are never substituted for `Scope.UserID`, and caller-supplied
User, Agent, and Run dimensions remain unchanged.

The new Workflow factories are independent configuration adapters. They do not
require ToolCall or translate Workspace Toolkit policy into provider-native
tools.
