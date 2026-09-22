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
Workflow and RuntimeProfile snapshot.

Each run resolves the current Peer RuntimeProfile Tool binding snapshot. The shared Agent receives one `genx.ToolInvoker`; every Transform resolves Tools from its own context, so concurrent Peers do not share Tool definitions, arguments or results even when they share the Agent.

Flowcraft, Eino, DashScope Realtime, and Doubao Realtime Duplex factories inject
the same interface into their existing Transformer configuration. Provider
ToolCall IDs and continuation stay inside the Transformer. AgentHost dispatches canonical Resource names to `http_request`; Tool control traffic stays inside the Transformer.

OpenAI Responses use a request-scoped direct Workspace attachment through the same canonical Resolver and shared Runtime Registry. It does not read or update the PeerRun selection. Server-side HTTP Tools keep normal Workflow policy. A bounded History observer returns the exact persisted assistant entry used by the Response projection.

When an assistant route ends with a provider or runtime error EOS, the Peer output adapter forwards the original EOS unchanged and emits one structured Server error record with Peer, active Workspace, stream, error-code, and retryability correlation. The expected `interrupted` replacement EOS is a control event and is not logged as a failure. Logging does not fail the long-lived output consumer, add another EOS, or prevent a later turn or Workspace reload.
