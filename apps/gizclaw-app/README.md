# GizClaw App

`apps/gizclaw-app` is the Flutter mobile client for GizClaw. The generated
project targets iOS and Android.

## Current Scope

- Configure a GizClaw server and keep the generated device identity in the
  platform secure store.
- Browse Flowcraft, Doubao, and translation workflows and their workspaces.
- Create and activate workspaces, switch between push-to-talk and realtime
  input, and view or replay workspace history.
- Manage friend invitations and friends, create groups, and open their chatroom
  workspaces.
- List and adopt pets, load their presentation and optional PIXA animation, and
  invoke pet actions.

Workflow, workspace, friend, group, and pet surfaces use the live GizClaw RPCs.
Drift caches the catalog and history data needed for responsive listing and
offline presentation. Prototype fixtures are limited to the demo controller and
widget tests.

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

The resolved app locale is also sent explicitly by workflow list and get RPCs
as `WORKFLOW_LOCALE_EN` or `WORKFLOW_LOCALE_ZH_CN`. Keep app strings out of RPC
payloads and do not cache localized workflow catalogs without their locale.

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
scan a GizClaw server QR code during setup or from the Identity screen.

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

Mobile presentation will likely need workflow display fields beyond the current
execution contract. Keep those fields in metadata/display-oriented schemas, not
inside workflow driver execution parameters.

Expected future contract work:

- Add display metadata for workflow cards, such as icon, banner image, category,
  featured rank, and short subtitle.
- Add a workflow filter to workspace listing so a workflow detail screen can
  load only its workspaces without client-side filtering.
- Decide whether mobile chat uses Peer OpenAI-compatible chat completions,
  workspace run status/history, or a dedicated chatroom workflow stream for each
  driver.
