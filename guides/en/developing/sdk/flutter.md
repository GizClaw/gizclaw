# Flutter SDK <Badge type="warning" text="WIP" />

> This page currently only describes the directory and boundaries of the SDK. The client, signaling, transport, RPC and PIXA modules are still to be expanded one by one.

`sdk/flutter/gizclaw` is the Dart/Flutter client SDK, which provides GizClaw client, signaling, WebRTC transport, RPC frame, method registry, PIXA and generated Protobuf message.

```text
sdk/flutter/gizclaw/
├── lib/gizclaw.dart       # Public library surface
├── lib/src/               # SDK implementation
├── lib/src/generated/     # Protobuf generated messages
├── test/                  # SDK behavior tests
└── tool/                  # Generation tools
```

The caller only relies on the public API exposed by `lib/gizclaw.dart` and does not directly rely on `lib/src/`. The source of truth for Schema and RPC methods is in [API Design](../api/overview).

The SDK connects through encrypted `/webrtc/v1/offer` signaling and
`giznet/v1/service/<service-id>` DataChannels. Its protocol core is plain Dart;
WebRTC transport is a Flutter adapter over `flutter_webrtc` and native platform
implementations. Generated Protobuf and method-registry files are committed, so
ordinary App builds do not require `protoc`; regeneration uses the package's
`protoc_plugin` development dependency.

```sh
cd sdk/flutter/gizclaw
flutter pub get
dart run tool/generate_rpc.dart
dart format lib test tool
flutter analyze
flutter test
```
