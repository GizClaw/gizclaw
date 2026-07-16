# Flutter SDK <Badge type="warning" text="WIP" />

> This page currently only describes the directory and boundaries of the SDK. The client, signaling, transport, RPC and PIXA modules are still to be expanded one by one.

`sdk/flutter/gizclaw` is the Dart/Flutter client SDK, which provides GizClaw client, signaling, WebRTC transport, RPC frame, method registry, PIXA and generated Protobuf message.

```text
sdk/flutter/gizclaw/
├── lib/gizclaw.dart       # Public library surface
├── lib/src/               # SDK implementation
├── lib/src/generated/     # Protobuf generated messages
├── test/                  # SDK behavior tests
└── tool/                  # Generation tools
```

The caller only relies on the public API exposed by `lib/gizclaw.dart` and does not directly rely on `lib/src/`. The source of truth for Schema and RPC methods is in [API Design](../api/overview).
