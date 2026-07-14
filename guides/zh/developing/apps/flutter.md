# Flutter App <Badge type="warning" text="WIP" />

> 本页目前只定义 Flutter App 与 SDK 的边界，页面结构、状态流和平台接线仍待逐项补充。

`apps/gizclaw-app` 是 GizClaw Flutter application。App 负责产品 UI、页面状态、用户交互和 Android/iOS platform wiring；连接、signaling、RPC 与 PIXA 等可复用能力由 `sdk/flutter/gizclaw` 提供。

```text
apps/gizclaw-app/
├── lib/       # Application UI 与 app-owned state
├── test/      # Widget 与 app behavior tests
├── android/   # Android platform wiring
└── ios/       # iOS platform wiring
```

App 不应复制 Flutter SDK 中的 protocol、transport 或 generated message。通用 SDK 能力应先进入 SDK，再由 App 消费。

编码与 lifecycle 规则见 [Dart 与 Flutter](/zh/coding-styles/dart-flutter)。
