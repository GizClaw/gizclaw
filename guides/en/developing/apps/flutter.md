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

The App owns the fixed Workflow Collections `assistants`, `translates`, `raids`, `story-teller`, and `role-play`, including their navigation labels, ordering, and icons. It requests each Collection explicitly and projects the RuntimeProfile-provided alias i18n for the current locale, with English and then the stable alias as fallback. RuntimeProfile does not translate the Collection or Profile itself.

Catalog refresh reconciles all five Collection snapshots atomically and rejects mixed RuntimeProfile revisions or duplicate aliases. Selecting a Workflow creates a new Workspace with its `collection` and `workflow_alias`, then enters it directly. The UI does not ask the user to choose a concrete Model or Voice; Workspace reload resolves current RuntimeProfile aliases. A Workspace whose alias is missing remains listed but is shown unavailable.

The Android application name and locale declaration are placed at `android/app/src/main/res`, and the iOS application name and
The permission description is placed at `Runner/*lproj/InfoPlist.strings`. Flutter and Android must be synchronized when adding a new language
and iOS three resources.
