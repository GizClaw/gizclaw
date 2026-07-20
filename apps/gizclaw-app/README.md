# GizClaw App

`apps/gizclaw-app` is the Flutter mobile client for GizClaw. The generated
project targets iOS and Android.

## Current Scope

- Configure a GizClaw server and keep the generated device identity in the
  platform secure store.
- Browse all supported workspaces through one Workspaces destination and create
  one from the App's fixed Workflow picker.
- Create and activate workspaces, switch between push-to-talk and realtime
  input, and view or replay workspace history.
- Manage friend invitations and friends, create groups, and open their chatroom
  workspaces.
- List and adopt pets, load their presentation and optional PIXA animation, and
  invoke pet actions.

Workspace, friend, group, and pet surfaces use the live GizClaw RPCs. The App's
Workflow aliases, ordering, i18n, icons, and typed creation parameters are fixed
release data matching `RuntimeProfile/default`; product navigation does not call
`server.workflow.list` or cache a server-provided Workflow catalog. Drift caches
Workspace and history data needed for responsive listing and offline
presentation. Prototype fixtures are limited to the demo controller and widget
tests.

## Development

```sh
flutter run
flutter analyze
flutter test
```

### Localization

The app ships English and Simplified Chinese UI resources. The language picker
is available before server setup and under Identity > App Settings. Its default
is System; unsupported system locales, including Traditional Chinese, resolve
to English.

Edit the ARB sources in `lib/l10n/`, then regenerate localizations before
building or testing:

```sh
flutter gen-l10n
```

Keep App strings in the local fixed catalog and out of RPC payloads.

For a development server, inject the ignored e2e identity at build time. The
iOS simulator can reach a server on the host through `127.0.0.1`:

```sh
flutter run \
  --dart-define=GIZCLAW_ENDPOINT=127.0.0.1:19820 \
  --dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>
```

For an Android emulator, use its host alias instead:

```sh
flutter run \
  --dart-define=GIZCLAW_ENDPOINT=10.0.2.2:19820 \
  --dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>
```

On a physical iOS or Android device, use the development machine's LAN address
and make sure the server listens on that interface.

The app does not ship with preset server endpoints. Add a server manually or
scan a GizClaw server QR code during setup or from the Identity screen. A
Desktop local Pod QR supplies the raw credential for the fixed application
token `app:com.gizclaw.opensource`; it is stored per Server only in platform
secure storage. The App does not expose arbitrary RegistrationToken editing or
selection.

GizClaw servers currently use plain HTTP. An endpoint without an explicit
scheme is therefore interpreted as `http://<host>:<port>`.

After each WebRTC connection is established, the app publishes its current
device information with `server.info.put` and serves `client.info.get` and
`client.identifiers.get` from the same snapshot. The Flutter SDK also dispatches
`client.tool.invoke`; the app returns method-not-found until it registers a
local tool handler.

Do not commit a private key or persist it in Drift. At runtime the app generates
or imports the device key through `flutter_secure_storage`; the endpoint is
stored separately in platform preferences.

Run commands from this directory:

```sh
cd apps/gizclaw-app
```

## Internal Testing

TestFlight and Google Play Internal publishing are owned by the private
[`GizClaw/deploy`](https://github.com/GizClaw/deploy) repository. This
repository owns the application identity and release-signing integration, but
does not store publishing credentials or run store-upload workflows.

The deployment repository checks out a requested GizClaw ref, validates the
fixed bundle/package identity, and builds the app with its committed release
credentials. See `credentials/mobile/README.md` in that repository for the
operator procedure.

## Integration Notes

Mobile presentation is keyed by the RuntimeProfile Workflow alias. Keep the
localized name, icon, banner, and other presentation metadata in the App's
local alias catalog rather than the Server Workflow execution contract.

The fixed selectable aliases are `doubao-realtime`, `translate-zh-en-auto`,
`translate-zh-ja`, `translate-zh-ko`, `translate-zh-es`, `chat`, `journey`, and
`murder-mystery`. `chatroom` is a fixed internal alias for Friend and Group
flows and is not offered by the picker. The legacy `ast-translate-zh-*` aliases
map to the corresponding localized translation cards for existing Workspaces,
but remain unavailable for new Workspace creation.
