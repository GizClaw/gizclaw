# Flutter SDK <Badge type="warning" text="WIP" />

> 本页目前只说明 SDK 的目录和边界，client、signaling、transport、RPC 与 PIXA 模块仍待逐项展开。

`sdk/flutter/gizclaw` 是 Dart/Flutter client SDK，提供 GizClaw client、signaling、WebRTC transport、RPC frame、method registry、PIXA 和生成的 Protobuf message。

```text
sdk/flutter/gizclaw/
├── lib/gizclaw.dart       # Public library surface
├── lib/src/               # SDK implementation
├── lib/src/generated/     # Protobuf generated messages
├── test/                  # SDK behavior tests
└── tool/                  # Generation tools
```

调用方只依赖 `lib/gizclaw.dart` 暴露的公共 API，不直接依赖 `lib/src/`。Schema 和 RPC method 的 source of truth 位于 [API Design](../api/overview)。
