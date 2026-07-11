# AgentHost GenX Wiring

This document describes the connection-level AgentHost structure.

## Core Idea

```text
PeerConn
└── wraps the live peer connection as GenX input and output streams
    └── AgentHost
        ├── chooses the current agent for this peer connection
        ├── starts the agent in the background
        ├── reloads the agent when workspace/config changes
        └── stops the agent when the peer connection closes
```

This is not a request-scoped RPC stream. The agent is a background runtime
attached to the live peer connection.

## GenX Interfaces

```go
type Stream interface {
	Next() (*MessageChunk, error)
	Close() error
	CloseWithError(error) error
}

type Transformer interface {
	Transform(ctx context.Context, pattern string, input Stream) (Stream, error)
}
```

## Peer Connection Stream

```go
type PeerConnStream interface {
	genx.Stream
}
```

`PeerConnStream` is the adapter that turns a live `PeerConn` into a GenX input
stream.

```go
type PeerConnGenXStream struct {
	Conn *PeerConn
}

func (s *PeerConnGenXStream) Next() (*genx.MessageChunk, error)
func (s *PeerConnGenXStream) Close() error
func (s *PeerConnGenXStream) CloseWithError(error) error
```

Input examples:

```text
incoming text event       -> genx.Text
incoming audio frame      -> genx.Blob
incoming control event    -> genx.MessageChunk{Ctrl: ...}
peer connection closed    -> io.EOF
```

## Peer Connection Output

```go
type PeerConnOutput interface {
	ConsumeAgentOutput(context.Context, genx.Stream) error
}
```

```go
type PeerConnOutput struct {
	Conn *PeerConn
}

func (o *PeerConnOutput) ConsumeAgentOutput(ctx context.Context, output genx.Stream) error
```

Output examples:

```text
genx.Text                 -> RPC/text event to peer
genx.Blob audio           -> PeerConn audio mixer
genx.MessageChunk tool    -> tool call handling
genx.MessageChunk Ctrl    -> stream routing/control
```

## Audio Mux

Each live `PeerConn` has one peer-level mixer.

```go
type PeerAudioMixer interface {
	CreateAudioTrack(...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error)
}
```

Each GenX audio stream should get its own mixer track.

```go
type GenXAudioMux struct {
	Mixer PeerAudioMixer
}

func (m *GenXAudioMux) AddStream(ctx context.Context, label string, stream genx.Stream) error
```

Mux shape:

```text
genx audio stream A -> mixer track A
genx audio stream B -> mixer track B
genx audio stream C -> mixer track C
                         └── PeerConn mixer
                             └── stamped opus output to peer
```

`TrackCtrl` controls per-stream gain, fade, and close. The peer mixer produces a
single mixed PCM stream, and `PeerConn` encodes that mixed stream to stamped
opus packets.

## Agent Runtime

The connection runtime owns the active AgentHost run for a peer connection.

```go
type AgentService struct {
	Host     *agenthost.Host
	Pattern  PatternSource
	Source   StreamSource
	Consumer StreamConsumer
}
```

```go
type PatternSource interface {
	AgentPattern(context.Context) (string, error)
}

type StreamSource interface {
	OpenAgentInput(context.Context) (genx.Stream, error)
}

type StreamConsumer interface {
	ConsumeAgentOutput(context.Context, genx.Stream) error
}
```

Runtime operations:

```go
func (s *AgentService) Reload(ctx context.Context) error
func (s *AgentService) Stop(ctx context.Context) error
```

`Reload` chooses the current workspace pattern, opens the peer connection stream,
calls `agenthost.Host.Transform`, starts output consumption in the background,
then stops the previous runtime.

## Agent Resolution

```go
type Host struct {
	Resolver    Resolver
	Registry    *Registry
	Coordinator Coordinator
}

type Resolver interface {
	Resolve(context.Context, string) (Spec, error)
}

type Factory interface {
	NewAgent(context.Context, Spec) (genx.Transformer, error)
}
```

```go
type Spec struct {
	Workspace apitypes.Workspace
	Workflow  apitypes.WorkflowDocument
	AgentType string
	Runtime   workspace.Runtime
	Toolkit   *ToolkitContext
}
```

Pattern resolution:

```text
peer connection
└── selected workspace name
    └── workspace.workflow_name
        └── workflow
            └── agent type
```

Agent type resolution:

```text
workspace.parameters["agent_type"]
└── else workflow apiVersion group
```

Toolkit resolution is policy based. `workflow.spec.toolkit.tool_ids` defines the
workflow-level allowlist; `workspace.toolkit.tool_ids` can further narrow it.
If both lists are present, AgentHost exposes their intersection. An empty
workspace list explicitly exposes no tools. When a Toolkit policy is configured,
AgentHost resolves it with the authenticated peer subject from `Reload`, so
ToolKit construction and invocation use the same `tool:use` ACL checks as direct
ToolKit APIs.

Flowcraft agents consume the resolved Toolkit through their Claw config and
runtime handlers. Each resolved Tool becomes one `agent.tools` entry using the
Tool's advertised name, description, and JSON input schema; the registered
handler calls back into ToolKit invocation with the same subject and effective
policy.

## PeerConn Wiring

```go
type PeerConn struct {
	Conn         *giznet.Conn
	Service      *PeerService
	AgentRuntime *AgentService
}
```

Connection lifecycle:

```text
PeerConn.serve()
├── serve peer service
├── serve RPC
├── serve mixed audio packets
├── start/reload AgentRuntime
└── stop AgentRuntime on close
```

## Control RPC

Peer run RPC methods are control methods for the connection-level background
runtime.

```text
server.run.agent.get
server.run.agent.set
server.run.reload
server.run.status
server.run.stop
```

They do not carry the agent input/output stream. The peer connection itself is
the stream.

```go
type PeerRunAgent struct {
	Active  *AgentSelection `json:"active,omitempty"`
	Pending *AgentSelection `json:"pending,omitempty"`
}

type AgentSelection struct {
	WorkspaceName string `json:"workspace_name"`
}
```

`server.run.agent.get` reads the peer's active and pending agent selection.

`server.run.agent.set` stores the peer's pending agent selection, such as the next
`workspace_name`. It does not switch the running agent by itself.

`server.run.reload` applies the pending agent selection and switches the
connection-level background agent runtime.
