# TypeScript SDK <Badge type="warning" text="WIP" />

> This page currently only explains the directory and contract boundaries of the SDK. The public surface, generation process and runtime behavior still need to be expanded one by one.

`sdk/js/gizclaw` Provides TypeScript client surface covering Admin HTTP, Public HTTP, RPC, signaling and Telemetry. `sdk/js/scripts` Stores the tools required to generate SDK surfaces from OpenAPI, Protobuf and method registry.

```text
sdk/js/
├── gizclaw/     # SDK package and generated client
└── scripts/     # contract generation and generated-output normalization
```

The source of truth of the generated content is located in [API Design](../api/overview), and the generated output cannot be directly maintained as a handwritten implementation.

Browser and Desktop clients connect through encrypted `/webrtc/v1/offer`
signaling and carry protobuf RPC envelopes, body frames, and EOS on the ordered
`giznet/v1/service/0` DataChannel. Before creating the offer,
`connectGiznetWebRTC` creates the packet DataChannel and an Opus-capable audio
transceiver; callers inject runtime-specific identity, crypto, and fetch
primitives.

`createWebRTCFetch` is the generated-client fetch adapter boundary. The current
WebRTC bridge maps HTTP requests to GizClaw RPC methods; it is not an arbitrary
HTTP proxy. Validate SDK changes with at least:

```sh
npm --prefix sdk/js test
npm --prefix sdk/js run gen:sdk
```
