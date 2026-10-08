# Firmware RPC

`Implementation file: services/runtime/peerresource/firmware.go`

A RegistrationToken may bind one canonical Firmware ID to a Peer. Devices do
not list or select a Firmware resource. They request one channel from the bound
resource through `server.firmware.get`.

Admin create/apply receives that immutable ID from the caller. Firmware has no
separate `name` or `spec.name`; its identity is needed only for the
Admin binding and is not exposed by Peer RPC.

The request contains only `channel`, one of `stable`, `beta`, or `develop`.
The response contains:

- the requested channel;
- the optional channel description;
- an absolute HTTPS URL for a `.tar.zlib` package (a tar archive compressed as one zlib stream);
- the SHA-256 and byte size of the exact compressed package.

Descriptions and URLs are limited to 1024 and 2048 UTF-8 bytes respectively.
Package size is positive and at most
`9007199254740991`, so JavaScript SDKs preserve the exact byte count.

The Peer downloads the URL directly and verifies the compressed bytes. GizClaw
does not fetch, unpack, proxy, upload, or stream firmware packages. A missing
binding, bound Firmware, channel package, or invalid channel returns an explicit
RPC error.

Firmware catalog and declarative channel ownership remain in
`services/device/firmware` and are managed through the Admin surface.

Response `version` is optional and comes from the selected channel's `package.version`: strict SemVer 2.0.0, at most 128 ASCII characters when present. Stored packages without versions remain available; the server omits the field without inferring a release. Protobuf field 6 is `optional string`: Go exposes a nullable pointer, JavaScript an optional property (`undefined` when absent), Dart `hasVersion()`, and C `has_version` with 129 bytes of string storage including NUL. Versions do not replace download integrity verification or the SHA-256 guard in OTA requests.

## Firmware metadata

Optional `spec.metadata` sits alongside `spec.slots`, independent of release channels.
It maps keys to arbitrary JSON values: objects, arrays, strings, numbers, booleans, or null.
Versions, URLs, and other fields belong to the consumer. The Server stores and reads
values without interpreting their business meaning.

```yaml
spec:
  slots:
    stable: {}
    beta: {}
    develop: {}
  metadata:
    modem:
      version: vendor-2026.10
      urls:
        - https://firmware.example.com/modem/ap.bin
        - https://firmware.example.com/modem/cp.bin
    label: Devkit
    enabled: true
    optional: null
```

`server.firmware.metadata.get` (138) accepts `{key: "modem"}` and returns `{key, value}`
from the caller's bound Firmware. `value` is compact UTF-8 JSON text; strings keep
JSON quotes and must also be decoded as JSON. A present null key returns `value: "null"`,
distinct from an absent key. Keys match exactly; dots are not paths. The request takes
no Firmware ID or channel. Missing bindings, Firmware records, or keys return `NOT_FOUND`.
Missing or invalid keys return `INVALID_ARGUMENT`; storage failures return `INTERNAL`
without internal diagnostics.

The map holds at most 64 keys, each matching `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`.
Each value's persisted JSON encoding is at most 65536 UTF-8 bytes. Go preserves raw
JSON numbers without float64 conversion, and RPC returns their complete text. Admin
create/put, resource apply/show, and Peer HTTP `/device/firmware` retain JSON values.
PUT/apply fully replace configuration; omitted or empty metadata removes previous entries.

Go uses `Client.GetFirmwareMetadata` with the original `rpcpb.FirmwareMetadataGetRequest`
and `FirmwareMetadataGetResponse`. Flutter uses `GizClawClient.getFirmwareMetadata(key)`
and `jsonDecode(response.value)`. JavaScript uses
`PeerRPCClient.getFirmwareMetadata(key, options?)` and `JSON.parse(response.value)`.
C uses `gzc_client_get_firmware_metadata(client, key, timeout_ms, options, &request)`
with the normal `gzc_client_poll`, `gzc_rpc_request_result`, and
`gzc_rpc_request_destroy` lifecycle. Decode the response's nanopb `value` callback as
JSON text, avoiding a large static value buffer. The start call copies the key,
so its caller does not need to retain the key buffer for the request lifetime.

## Core structure

| Symbol | Function |
| --- | --- |
| `FirmwareGet` | Validates the requested channel, resolves the Peer binding, and returns that channel package configuration. |
| `FirmwarePackage` | Admin-side external package contract: SemVer version, HTTPS URL, SHA-256, and compressed size. |
