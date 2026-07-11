# GizClaw Dart SDK

`sdk/flutter/gizclaw` is the Flutter client SDK for GizClaw peer connections
over WebRTC.

The SDK mirrors the existing JavaScript peer transport contract:

- encrypted `/webrtc/v1/offer` signaling;
- `giznet/v1/service/<service-id>` DataChannel labels;
- Peer RPC frame and protobuf envelopes from `docs/rpc_protocol.md`;
- service IDs from `docs/service_layout.md`;
- generated RPC method and payload metadata from `api/rpc/peer.proto`.

The protocol core is plain Dart, but WebRTC transport support is a Flutter
adapter over `flutter_webrtc` and native platform WebRTC implementations.

## Development

```sh
flutter pub get
dart run tool/generate_rpc.dart
dart format lib test tool
flutter analyze
flutter test
```

Generated protobuf and registry files are committed. Normal app builds do not
need `protoc`. Regeneration requires `protoc`; the Dart plugin is resolved from
the package's `protoc_plugin` development dependency.
