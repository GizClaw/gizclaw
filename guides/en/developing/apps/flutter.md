# Flutter App <Badge type="warning" text="WIP" />

> This page currently only defines the boundary between Flutter App and SDK. The page structure, status flow and platform wiring still need to be added one by one.

`apps/gizclaw-app` is GizClaw Flutter application. App is responsible for product UI, page status, user interaction and Android/iOS platform wiring; reusable capabilities such as connection, signaling, RPC and PIXA are provided by `sdk/flutter/gizclaw`.

```text
apps/gizclaw-app/
├── lib/       # application UI and app-owned state
├── test/      # widget and app behavior tests
├── android/   # Android platform wiring
└── ios/       # iOS platform wiring
```

Apps should not copy protocol, transport, or generated messages from the Flutter SDK. Common SDK capabilities should first enter the SDK and then be consumed by the App.

For coding and lifecycle rules, see [Dart and Flutter](/en/coding-styles/dart-flutter).

## Internationalization

App uses Flutter `gen_l10n` and ARB as the only source of page copy, English
`app_en.arb` is a template, and the simplified Chinese resource is `app_zh.arb` / `app_zh_CN.arb`. Add or
After modifying the copy, run it at `apps/gizclaw-app`:

```sh
flutter gen-l10n
```

Language preferences are held and persisted by the app itself, supporting "following the system", English and Simplified Chinese.
The language selector must also be able to be opened when the server is not configured. When following the system, only English and Simplified Chinese are mapped as
Supported locales; Traditional Chinese and other unsupported languages fall back to English.

The currently valid locale determines both the UI and workflow RPC locale: English must be passed explicitly
`WORKFLOW_LOCALE_EN`, Simplified Chinese must be passed explicitly `WORKFLOW_LOCALE_ZH_CN`, and cannot be omitted or
Pass `WORKFLOW_LOCALE_UNSPECIFIED`. Switching languages refreshes the workflow catalog; in Drift
Localized workflow data must be recorded in the corresponding locale, and old caches that do not match the locale are ignored when reading.
And fallback with stable `Workflow.name`. Asynchronous refresh must also verify the locale generation before writing to the cache.
Prevent earlier requests from overwriting the switched language.

The Android application name and locale declaration are placed at `android/app/src/main/res`, and the iOS application name and
The permission description is placed at `Runner/*lproj/InfoPlist.strings`. Flutter and Android must be synchronized when adding a new language
and iOS three resources.
