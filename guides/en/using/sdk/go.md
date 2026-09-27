# Go SDK <Badge type="warning" text="WIP" />

This page will explain Go SDK installation, client initialization, authentication, connection, API/RPC calls and error handling.

## Handshake admission credentials

`gizwebrtc.DialConfig.Credential` directly accepts `*giznetpb.AdmissionCredential` with
`Version`, `Type`, and `Value`. The GizClaw helper `gizcli.RegistrationTokenCredential(token)`
constructs version 1 and type `gizclaw.com/registration_token`. The SDK protobuf-encodes the structure; nil
means no credential. See [Giznet](../../developing/giznet#signaling-admission-credentials) for encoding
and bounds. Admission binds no resources; still invoke `server.register` after connecting.
Admission conditions belong to [Security Policy](../../developing/gizclaw/server/security-policy).

`RegistrationTokenCredential` returns `(credential, error)`; handle the error before passing the structure to Dial. `gizcli.RegistrationTokenCredentialType` is the exported built-in type constant. A 512-byte UTF-8 value is accepted; 513 bytes fail during helper construction.

## MHS v0 HWDs

Install `DeviceControlHandlers.ReadMhsHwd` and `WriteMhsHwd` with `gizcli.Client.HandleDeviceControl`. Handlers receive `rpcpb.ClientMhsV0ReadRequest` or `ClientMhsV0WriteRequest`; the `ClientHwd` registry and helpers such as `rpcapi.ClientHwdReadResponseMessage` / `ClientHwdWriteRequestFromBytes` select the HWD-specific protobuf. Each call addresses one `id`. Wifi, ble, modem, battery and mic are read-only; display, led and speaker can be written. Return `ErrDeviceResourceNotFound` for absent physical hardware.

The RuntimeProfile manifest is readable offline and declares only instance IDs and HWD types. Drivers enforce safety limits and return actual applied values in write responses. See [Public API](/en/developing/api/http/public) and the [provider contract](/en/developing/api/proto/rpc/client-provided-to-server).

## tool/v0 procedures

Device providers install handlers for the predefined `ClientTool` values they implement. `client.tool.v0.list` reports that installed subset; typed control calls use the single `client.tool.v0.invoke` RPC. HWD instances use the bound RuntimeProfile MHS v0 manifest and its read/write calls.
