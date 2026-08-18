# OpenAI HTTP

`Implementation file: server_openai_http.go`

Assemble the Peer-scoped OpenAI-compatible handler for the ordinary Server HTTP entry, and connect the public-login session to the corresponding RuntimeProfile resource view.

The Server authenticates the primary session before stripping `/openai`. The retained handler in `PeerService` then applies an exact method/path allowlist, binds the verified canonical Peer ID and request-scoped resources, and delegates the four standard operations to AI Server Shell. `/v1/voices` remains a GizClaw handler outside the Shell. Unsupported paths return `404` only after ingress authentication.

Bearer and cookie values are never the backend identity. The Shell authenticator reads the verified context binding and fails closed if it is absent. CORS and preflight behavior remain owned by the outer Server handler.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `peerOpenAIHTTPHandler` | Authenticate the primary session and bind its immutable registration snapshot before dispatch. |
| `openAIProtocolHandler` | Lazily construct and retain one Shell router for a `PeerService` handler graph, keeping unrelated Server startup independent of OpenAI schema parsing. |

The same authenticated composition exposes Workspace-backed Conversations and Responses. One process-shared Response runtime serializes active work per Workspace, persists terminal state before returning terminal JSON/SSE, and converts abandoned `in_progress` records to a safe restart failure on retrieval. Server shutdown stops and joins this runtime before Workspace and Agent stores are closed.
