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

Response `version` comes from the selected channel's `package.version`, a SemVer 2.0.0 release version of at most 128 ASCII characters. The C SDK reserves 129 bytes including NUL. The version does not replace download integrity verification or the SHA-256 guard in OTA requests. Stored packages without a valid version produce an internal RPC error; operators must repair their configuration with a real version through Admin PUT. An older protobuf server can omit the added wire field, but the updated service never returns an empty version in a successful response.

## Core structure

| Symbol | Function |
| --- | --- |
| `FirmwareGet` | Validates the requested channel, resolves the Peer binding, and returns that channel package configuration. |
| `FirmwarePackage` | Admin-side external package contract: SemVer version, HTTPS URL, SHA-256, and compressed size. |
