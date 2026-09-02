# Flutter SDK <Badge type="warning" text="WIP" />

> This page describes the directories, boundaries, and validation of the two Dart packages. The client, signaling, transport, RPC, and PIXA modules inside `gizclaw` are still to be expanded one by one.

`sdk/flutter/` holds two packages, one per side of GizClaw:

- `sdk/flutter/gizclaw` is the device-side Dart/Flutter SDK: GizClaw client, signaling, WebRTC transport, RPC frame, method registry, PIXA, and generated Protobuf messages. It covers the same side as `sdk/c/gizclaw`.
- `sdk/flutter/gizclaw_control` is the controller-side pure Dart SDK that calls `/gizclaw/v1/*` over HTTPS with an API key. It has no Flutter, `flutter_webrtc`, or Protobuf dependency and depends on `http` only.

```text
sdk/flutter/
├── gizclaw/
│   ├── lib/gizclaw.dart          # Public library surface
│   ├── lib/src/                  # SDK implementation
│   ├── lib/src/generated/        # Protobuf generated messages
│   ├── test/                     # SDK behavior tests
│   └── tool/                     # Generation tools
└── gizclaw_control/
    ├── lib/gizclaw_control.dart  # Public library surface
    ├── lib/src/client.dart       # GizClawControlClient: routes, bearer header, timeout
    ├── lib/src/errors.dart       # GizClawControlException and error classification
    ├── lib/src/models.dart       # Hand-written contract models (fromJson/toJson)
    ├── lib/src/json.dart         # Strict JSON readers
    └── test/                     # MockClient-driven route and error-mapping tests
```

Callers depend only on the public API exposed by each package's `lib/<package>.dart`, never on `lib/src/`. The source of truth for schemas and RPC methods is [API Design](../api/overview).

## `gizclaw`

The SDK connects through encrypted `/webrtc/v1/offer` signaling and
`giznet/v1/service/<service-id>` DataChannels. Its protocol core is plain Dart;
WebRTC transport is a Flutter adapter over `flutter_webrtc` and native platform
implementations. Generated Protobuf and method-registry files are committed, so
ordinary app builds do not require `protoc`; regeneration uses the package's
`protoc_plugin` development dependency.

```sh
cd sdk/flutter/gizclaw
flutter pub get
dart run tool/generate_rpc.dart
dart format lib test tool
flutter analyze
flutter test
```

## `gizclaw_control`

Models are hand-written from `api/http/peer.json` and the `api/http/shared.json` schemas it references; there is no generation step, so a contract change updates `models.dart` and its tests together. Decoding throws `FormatException` for a required field of the wrong type (the client maps it to `malformedResponse`), ignores unknown keys, and keeps `raw` on the open-ended `PeerStatus` and `DeviceInfo` schemas. The error `kind` first matches `DEVICE_*` on the body's `error.code`, then classifies by HTTP status; the code constants are owned by `pkgs/gizclaw/peer_service_serve_peer_http_device_control.go`.

```sh
cd sdk/flutter/gizclaw_control
dart pub get
dart format --output=none --set-exit-if-changed lib test
dart analyze --fatal-infos
dart test
```

The CI `quality` and `flutter` jobs run these checks for both packages, and the handwritten Dart files listed by `go run ./tools/quality files -language=dart` include `gizclaw_control`.
