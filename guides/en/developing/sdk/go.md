# Go SDK <Badge type="warning" text="WIP" />

> This page currently only describes the positioning and scope of the SDK. The public client, connection, RPC, stream and telemetry modules are still to be expanded one by one.

`sdk/go/gizcli` Provides the GizClaw client surface used by Go callers, including connections, security policies, resources, Peer stream, RPC, Telemetry and WebRTC access.

Go SDK is client-facing boundary and does not have server domain behavior. API and RPC methods come from [API Design](../api/overview); when modifying the contract, the surface, SDK implementation and test must be generated simultaneously.

Caller-scoped Peer deletion is exposed as `Client.DeletePeer`. A successful response is terminal for the current Peer connection, so the caller must reconnect before issuing more work.

## Context connections and Admin Resources

Two subpackages provide the pure Go surface shared by the CLI and the Terraform provider. They build with `CGO_ENABLED=0` for darwin/linux on amd64/arm64 and do not depend on `cmd/...`, the embedded Console, or native model runtimes.

- `sdk/go/gizcli/contextconn`: `ConfigDir` returns the CLI context root; `LoadContext` reads `<ConfigDir>/<context>/config.yaml` (the current context when the name is empty) and replaces `server.endpoint` with `Options.Endpoint`, which must be `https://host[:port]`; `Dial` fetches server-info and prepares the WebRTC client; `Connect` connects, starts the client-side Peer services, and waits until Peer HTTP answers; its `context.Context` cancels the server-info requests, the WebRTC dial, and the readiness wait. The caller owns the returned client and must `Close` it.
- `sdk/go/gizcli/adminresource`: `Client` wraps Admin HTTP `ApplyResource`, `GetResource`, and `DeleteResource`; non-success responses return `*ResponseError`, whose text stays `CODE: message`, and `IsNotFound` recognizes only `<SCOPE>_NOT_FOUND` error codes. `GetResources` runs at most 8 reads on one connection and returns one result per reference in input order. `PrepareManifest`, `DecodeManifest`, and `FormatForPath` implement the JSON/YAML, `${VAR}` expansion, and `<Kind>Resource` alias rules of `gizclaw admin apply|validate`.

[contextconn API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn) · [adminresource API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource)

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/sdk/go/gizcli)

## Initial audio BOS and RTP

The Peer Event Stream and fixed Opus RTP track are independent channels. Sending
BOS successfully does not mean that the receiver has processed it. `PeerStream`
buffers RTP payloads before the first audio BOS, emits BOS to its caller first,
then emits the buffered audio in arrival order with that StreamID and label.
Text events do not release buffered audio. The buffer is limited to 64 packets
or 128 KiB, with a one-second BOS deadline starting at the first packet. Overflow
or timeout terminates the stream with an error; closing releases the buffer.

This does not change the wire schema, device firmware, or other language SDKs,
and adds no downlink ACK. RTP payloads have no logical StreamID, so a late packet
from an earlier epoch cannot be distinguished from the next epoch's first packet.
Buffering therefore applies only to the initial audio binding; packets after EOS
are not reassigned to the next BOS and remain visible to caller boundary checks.
Existing EOS draining and audio integrity assertions remain unchanged.
