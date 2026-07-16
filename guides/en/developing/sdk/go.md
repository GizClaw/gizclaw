# Go SDK <Badge type="warning" text="WIP" />

> This page currently only describes the positioning and scope of the SDK. The public client, connection, RPC, stream and telemetry modules are still to be expanded one by one.

`sdk/go/gizcli` Provides the GizClaw client surface used by Go callers, including connections, security policies, resources, Peer stream, RPC, Telemetry and WebRTC access.

Go SDK is client-facing boundary and does not have server domain behavior. API and RPC methods come from [API Design](../api/overview); when modifying the contract, the surface, SDK implementation and test must be generated simultaneously.

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)
