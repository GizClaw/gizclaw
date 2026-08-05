# Firmware RPC

`Implementation file: services/runtime/peerresource/firmware.go`

A RegistrationToken may bind one canonical Firmware ID to a Peer. Devices do
not list or select a firmware release line. They request one channel from the
bound release line through `server.firmware.get`.

The request contains only `channel`, one of `stable`, `beta`, `develop`, or
`pending`. The response contains:

- the peer-visible firmware name and requested channel;
- the optional channel description;
- an absolute HTTPS URL for a `.tar.zlib` package (a tar archive compressed as one zlib stream);
- the SHA-256 and byte size of the exact compressed package.

Peer-visible names, descriptions, and URLs are limited to 256, 1024, and 2048
UTF-8 bytes respectively. Package size is positive and at most
`9007199254740991`, so JavaScript SDKs preserve the exact byte count.

The Peer downloads the URL directly and verifies the compressed bytes. GizClaw
does not fetch, unpack, proxy, upload, or stream firmware packages. A missing
binding, bound Firmware, channel package, or invalid channel returns an explicit
RPC error.

Firmware catalog and release-channel ownership remain in
`services/device/firmware` and are managed through the Admin surface.

## Core structure

| Symbol | Function |
| --- | --- |
| `FirmwareGet` | Validates the requested channel, resolves the Peer binding, and returns that channel package configuration. |
| `FirmwarePackage` | Admin-side external package contract: HTTPS URL, SHA-256, and compressed size. |
