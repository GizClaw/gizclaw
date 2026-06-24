# Peer Run State

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/18

This package is reserved for foundation-level peer-owned runtime state.

Planned scope:

- `server.status.{get,put}` state primitives.
- `server.run.agent.{get,set}` active/pending agent selection primitives.
- Shared validation and storage shape used by later Server Service modules.

This package should not implement AgentHost or business-domain behavior.
