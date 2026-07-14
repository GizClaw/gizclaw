# Peer Telemetry Protobuf

This package contains the generated Go protobuf types for
`api/proto/telemetry/peer_telemetry.proto`.

Regenerate after changing the schema:

```sh
go generate ./pkgs/gizclaw/api/telemetry
```

The schema in `api/telemetry` is the cross-language source of truth. Keep field
numbers append-only, never reuse deleted field numbers, and do not change the
type or semantics of existing fields.
