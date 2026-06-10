# Agent Runtime

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/20

This package is reserved for connection-level AgentHost runtime wiring.

Planned scope:

- `peer.run.agent.{get,set}` runtime integration.
- `peer.run.{reload,status,stop}` runtime lifecycle.
- `GearConn` to GenX stream adapters.
- AgentHost start, reload, status, and stop wiring.

Core AgentHost resolution stays in `pkg/gizclaw/agenthost`.
