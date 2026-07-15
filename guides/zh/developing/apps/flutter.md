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

当前有效 locale 同时决定 UI 和 workflow RPC locale：英文必须显式传
`WORKFLOW_LOCALE_EN`，简体中文必须显式传 `WORKFLOW_LOCALE_ZH_CN`，不得省略或
传 `WORKFLOW_LOCALE_UNSPECIFIED`。切换语言会刷新 workflow catalog；Drift 中的
本地化 workflow 数据必须记录对应 locale，读取时忽略 locale 不匹配的旧缓存，
并以稳定的 `Workflow.name` 回退。异步刷新还必须在写缓存前校验 locale generation，
避免较早请求覆盖切换后的语言。

Android 的应用名和 locale 声明放在 `android/app/src/main/res`，iOS 的应用名与
权限说明放在 `Runner/*lproj/InfoPlist.strings`。新增语言时必须同步 Flutter、Android
和 iOS 三处资源。
