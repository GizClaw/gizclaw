/// Controller-side client for the GizClaw API-key HTTP API.
///
/// The package talks to `/gizclaw/v1/*` over HTTPS with
/// `Authorization: Bearer gizclaw_sk_v1_...` and has no Flutter, WebRTC, or
/// Protobuf dependency. The device side of GizClaw lives in the sibling
/// `gizclaw` package.
library;

export 'src/client.dart';
export 'src/errors.dart';
export 'src/models.dart';
