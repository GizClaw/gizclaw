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

## Development and validation

Run from `apps/gizclaw-app`:

```sh
flutter run
flutter analyze
flutter test
```

A development Server identity may be supplied with
`--dart-define=GIZCLAW_ENDPOINT=<host:port>` and
`--dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>`. The iOS simulator
can reach `127.0.0.1`; Android emulators use `10.0.2.2`; physical devices need a
reachable LAN address. Private keys only belong in platform secure storage and
must not be committed or stored in Drift. Endpoints are stored separately in
platform preferences.

After WebRTC connects, the App publishes current device information with
`server.info.put` and serves `client.info.get` and `client.identifiers.get` from
the same snapshot. The Flutter SDK also dispatches `client.tool.invoke`; the App
returns method-not-found until it registers a local tool handler.

## iOS launch image

`ios/Runner/Assets.xcassets/LaunchImage.imageset/LaunchImage-iPad@2x.png` is the
canonical text-free, logo-free RGB raster master at `2048x2732`. iPhone images
use a centered narrow crop. Run in that imageset directory:

```sh
sips -c 2732 1261 LaunchImage-iPad@2x.png \
  --out /tmp/LaunchImage-iPhone-crop.png
sips -z 1864 860 /tmp/LaunchImage-iPhone-crop.png \
  --out LaunchImage-iPhone@2x.png
sips -z 2796 1290 /tmp/LaunchImage-iPhone-crop.png \
  --out LaunchImage-iPhone@3x.png
```

Committed dimensions are `860x1864`, `1290x2796`, and `2048x2732`; all remain
RGB PNG. `LaunchScreen.storyboard` uses aspect fill and `#F5F6F2` is only the
fallback behind the opaque artwork.
