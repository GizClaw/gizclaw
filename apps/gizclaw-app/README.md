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

For a development server, inject the ignored e2e identity at build time:

```sh
flutter run \
  --dart-define=GIZCLAW_ENDPOINT=127.0.0.1:19820 \
  --dart-define=GIZCLAW_PRIVATE_KEY=<development-private-key>
```

Do not commit a private key or persist it in Drift. At runtime the app generates
or imports the device key through `flutter_secure_storage`; the endpoint is
stored separately in platform preferences.

Run commands from this directory:

```sh
cd apps/gizclaw-app
```

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
