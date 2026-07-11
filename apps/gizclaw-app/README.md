# GizClaw App

`apps/gizclaw-app` is the Flutter mobile client for GizClaw. The generated
project targets iOS and Android.

## Current Scope

- Browse workflow cards in a Game Center-style home screen.
- Open a workflow detail screen and choose a workspace.
- Enter a workspace chat screen.
- Show starter Chatroom, Pet, and Me tabs.

Workflow and workspace surfaces read from a Drift cache populated through the
GizClaw WebRTC client. Collection, group chat, friends, and pet content remain
prototype fixtures until their server contracts are designed.

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

Do not commit a private key or persist it in Drift. Production enrollment and
secure identity storage are outside the prototype connection flow.

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
