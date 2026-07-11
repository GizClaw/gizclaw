# GizClaw App

`apps/gizclaw-app` is the Flutter mobile client for GizClaw. The generated
project targets iOS and Android.

## Current Scope

- Browse workflow cards in a Game Center-style home screen.
- Open a workflow detail screen and choose a workspace.
- Enter a workspace chat screen.
- Show starter Chatroom, Pet, and Me tabs.

The current UI uses local fixture data. Runtime integration should attach this
shell to the GizClaw WebRTC client path instead of reimplementing the protocol
in the widget layer.

## Development

```sh
flutter run
flutter analyze
flutter test
```

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
