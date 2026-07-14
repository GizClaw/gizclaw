# Dart 与 Flutter

本规范适用于 `sdk/flutter/gizclaw` 中的 Dart/Flutter SDK，以及 `apps/gizclaw-app` 中的 Flutter App。SDK 拥有 GizClaw contract、transport 与可复用 client 能力；App 负责产品 UI、页面状态和平台集成，不能复制 SDK 已经提供的协议实现。

## Dart 基础规则

- 使用 `dart format` 产生的标准格式，不手工维护与 formatter 冲突的排版。
- SDK 遵循 `package:lints/recommended.yaml`，App 遵循 `package:flutter_lints/flutter.yaml`；新增 lint suppression 必须限制到最小范围并说明原因。
- library、file、variable 和 function 使用 `lower_snake_case` 或 `lowerCamelCase` 对应的 Dart 官方惯例；type、enum 和 extension 使用 `UpperCamelCase`。
- 默认保持符号私有，只导出 SDK 使用者实际需要的稳定 surface。公共 API 从 `lib/gizclaw.dart` 统一暴露，调用方不应依赖 `lib/src/`。
- immutable value 优先使用 `final`；只有确实需要重新赋值时使用可变变量。
- 不使用 `dynamic` 或 unchecked cast 绕过外部 payload、platform channel 和生成 message 的边界检查。

## SDK 边界

- OpenAPI、Protobuf 和 RPC Schema 是生成 Dart message 与 method registry 的 source of truth；`lib/src/generated/` 只能由生成流程更新。
- signaling、WebRTC transport、RPC frame 和 payload codec 各自维护单一职责，不在 App 中重新拼装协议 frame 或复制 method name。
- SDK public method 应返回稳定的 Dart type，并清楚表达错误、取消与关闭语义；不要把 provider、WebRTC plugin 或生成代码的内部类型无必要地泄漏给 App。
- stream 必须定义 subscription、error、done 与 cancellation 行为。创建 `StreamController`、peer connection、data channel 或 subscription 的 owner 也必须负责关闭。
- `Future` 必须被 `await`、返回或显式处理；异步 callback 中的错误不能静默丢失。
- 网络、RPC、signaling 和 codec 输入均视为不可信，必须处理 malformed frame、未知 method、错误 payload、重复关闭和连接中断。

## Flutter App

- Widget 只负责展示和交互；协议、transport、重试与资源解析应留在 SDK 或明确的 application service。
- `State` 创建的 controller、subscription、animation、timer 和 SDK connection 必须在 `dispose` 中按所有权释放。
- `await` 之后访问 `BuildContext` 或调用 `setState` 前检查 `mounted`，避免页面销毁后的更新。
- 页面必须明确 loading、empty、error、success、offline 和 permission-denied 状态，不能用无限 loading 隐藏失败。
- build method 保持无副作用；网络请求、subscription 建立和持久化写入不能由重复 build 隐式触发。
- platform-specific 行为通过明确的 adapter 或 plugin boundary 管理，并分别验证 Android、iOS 与测试环境的差异。
- 交互组件应提供稳定的语义、可点击范围、disabled 状态、focus/keyboard 行为和可测试的 widget key 或 semantics。

## 状态与数据流

- 状态的 source of truth 必须唯一。Widget local state 只保存局部展示状态，连接、session 和共享资源状态由其明确 owner 管理。
- 不把 `BuildContext`、Widget 或 `State` 对象传入 SDK。
- 对实时 stream 应明确 ordering、reconnect、duplicate event、terminal state 和 backpressure 的处理方式。
- 昂贵 decode、音视频转换或大资源处理不能阻塞 UI isolate；需要时使用 isolate 或 native/plugin 能力，并定义取消和资源释放。

## 测试与验证

- SDK test 覆盖 signaling、frame、codec、method registry、transport、错误路径和连接关闭。
- Widget test 覆盖 loading、error、empty、交互与销毁后的异步回调；涉及真实 plugin 或平台生命周期时使用 integration test。
- Schema 或生成代码变化后重新运行生成工具，并确认生成 diff 与 source contract 一致。
- 修改 SDK 时运行：

```sh
dart format sdk/flutter/gizclaw
flutter analyze sdk/flutter/gizclaw
flutter test sdk/flutter/gizclaw
```

- 修改 App 时运行：

```sh
dart format apps/gizclaw-app
flutter analyze apps/gizclaw-app
flutter test apps/gizclaw-app
```
