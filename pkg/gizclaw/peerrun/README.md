# Peer Run State

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/18

This package is reserved for foundation-level peer-owned runtime state.

Planned scope:

- `peer.status.{get,put}` state primitives.
- `peer.run.agent.{get,set}` active/pending agent selection primitives.
- Shared validation and storage shape used by later Gear Service modules.

This package should not implement AgentHost or business-domain behavior.
