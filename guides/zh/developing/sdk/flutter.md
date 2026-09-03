# Flutter SDK <Badge type="warning" text="WIP" />

> 本页说明两个 Dart package 的目录、边界与验证方式；`gizclaw` 内部的 client、signaling、transport、RPC 与 PIXA 模块仍待逐项展开。

`sdk/flutter/` 下有两个 package，分别对应 GizClaw 的两侧：

- `sdk/flutter/gizclaw` 是设备端 Dart/Flutter SDK，提供 GizClaw client、signaling、WebRTC transport、RPC frame、method registry、PIXA 和生成的 Protobuf message。它与 `sdk/c/gizclaw` 覆盖同一侧能力。
- `sdk/flutter/gizclaw_control` 是控制端纯 Dart SDK，用 API Key 通过 HTTPS 调用 `/gizclaw/v1/*`。它不依赖 Flutter、`flutter_webrtc` 或 Protobuf，只依赖 `http`。

```text
sdk/flutter/
├── gizclaw/
│   ├── lib/gizclaw.dart          # Public library surface
│   ├── lib/src/                  # SDK implementation
│   ├── lib/src/generated/        # Protobuf generated messages
│   ├── test/                     # SDK behavior tests
│   └── tool/                     # Generation tools
└── gizclaw_control/
    ├── lib/gizclaw_control.dart  # Public library surface
    ├── lib/src/client.dart       # GizClawControlClient：route、bearer header、timeout
    ├── lib/src/errors.dart       # GizClawControlException 与错误分类
    ├── lib/src/models.dart       # 手写 contract model（fromJson/toJson）
    ├── lib/src/json.dart         # 严格 JSON reader
    └── test/                     # MockClient 驱动的 route 与错误映射测试
```

调用方只依赖各 package 的 `lib/<package>.dart` 暴露的公共 API，不直接依赖 `lib/src/`。Schema 和 RPC method 的 source of truth 位于 [API Design](../api/overview)。

## `gizclaw`

SDK 通过 encrypted `/webrtc/v1/offer` signaling 和
`giznet/v1/service/<service-id>` DataChannel 与 GizClaw 连接。Protocol core 是纯 Dart；
WebRTC transport 是基于 `flutter_webrtc` 与各 native platform implementation 的 Flutter
adapter。Generated Protobuf 与 method registry 提交到仓库，普通 App build 不需要
`protoc`；regeneration 需要 package 的 `protoc_plugin` development dependency。

```sh
cd sdk/flutter/gizclaw
flutter pub get
dart run tool/generate_rpc.dart
dart format lib test tool
flutter analyze
flutter test
```

## `gizclaw_control`

model 按 `api/http/peer.json` 与其引用的 `api/http/shared.json` schema 手写，没有生成步骤：contract 变化时同步修改 `models.dart` 与测试。解码规则是必填字段类型错误抛 `FormatException`（client 映射为 `malformedResponse`），未知字段忽略，`PeerStatus` 与 `DeviceInfo` 这类开放 schema 额外保留 `raw`。错误 `kind` 先按 body 的 `error.code` 匹配 `DEVICE_*`，再按 HTTP status 分类；code 常量以 `pkgs/gizclaw/peer_service_serve_peer_http_device_control.go` 为准。

`send()` 是 typed method 之外的 escape hatch：它向绝对 path 发送一次带 bearer 的请求，
返回 status 与解码后的 body 而不是抛出，用于访问本 package 尚未建模的 route；调用方可用
`classifyGizClawControlError` 自行分类。

```sh
cd sdk/flutter/gizclaw_control
dart pub get
dart format --output=none --set-exit-if-changed lib test
dart analyze --fatal-infos
dart test
```

CI 的 `quality` 与 `flutter` job 对两个 package 分别执行上述检查；`go run ./tools/quality files -language=dart` 列出的手写 Dart 文件同样覆盖 `gizclaw_control`。
