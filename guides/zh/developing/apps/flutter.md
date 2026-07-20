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

## 国际化

App 使用 Flutter `gen_l10n` 和 ARB 作为页面文案的唯一来源，英文
`app_en.arb` 是模板，简体中文资源为 `app_zh.arb` / `app_zh_CN.arb`。新增或
修改文案后在 `apps/gizclaw-app` 运行：

```sh
flutter gen-l10n
```

语言偏好由 App 自己持有并持久化，支持“跟随系统”、English 和简体中文。
未配置服务器时也必须能打开语言选择器。跟随系统时只把英文和简体中文映射为
支持的 locale；繁体中文及其他未支持语言回退到英文。

App 固定拥有 `assistants`、`translates`、`raids`、`story-teller` 和 `role-play`
五个 Workflow Collection，包括菜单名称、顺序与图标。App 必须逐个显式请求
Collection，并按当前 locale 投影 RuntimeProfile 返回的 alias i18n；缺失时先回退英文，
最后回退稳定 alias。RuntimeProfile 不翻译 Collection 或 Profile 自己。

Catalog refresh 必须原子协调五个 Collection snapshot，并拒绝混合 RuntimeProfile
revision 或重复 alias。用户选择 Workflow 后，App 使用它的 `collection` 与
`workflow_alias` 新建 Workspace，然后直接进入，不再要求选择真实 Model 或 Voice。
Workspace reload 会重新解析当前 RuntimeProfile alias；alias 缺失的 Workspace 仍在
列表中，但显示为 unavailable。

Android 的应用名和 locale 声明放在 `android/app/src/main/res`，iOS 的应用名与
权限说明放在 `Runner/*lproj/InfoPlist.strings`。新增语言时必须同步 Flutter、Android
和 iOS 三处资源。
