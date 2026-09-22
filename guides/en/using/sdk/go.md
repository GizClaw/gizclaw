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

## MHS v0 hardware states

Install `DeviceControlHandlers.ReadMhsStates` and `WriteMhsStates` with `gizcli.Client.HandleDeviceControl`; signatures use original `rpcpb.ClientMhsV0*` messages. Return `ErrDeviceResourceNotFound` for absent hardware or `rpcapi.Error{Code: rpcapi.StatusCodeFailedPrecondition}` when current conditions prohibit a write.

This is GizClaw's MHS-inspired pre-standard v0, with no official compatibility claim. Manifests work offline; reads/writes allow at most 32 unique keys. Drivers validate the whole batch and enforce safety limits. See [Public API](/en/developing/api/http/public) and the [provider contract](/en/developing/api/proto/rpc/client-provided-to-server).

## Deprecated hardware-state interfaces

`client.device.volume.set` (101), `client.device.settings.get` (128) and `client.device.settings.set` (129) are deprecated in favor of `client.mhs.v0.write`, `client.mhs.v0.read` and `client.mhs.v0.write`, respectively. Legacy entry points remain compatible. The [migration table](/en/developing/api/overview#mhs-v0-migration) lists recommended product manifest keys. Removal waits for both firmware and control apps to migrate; no date is set.

Device providers: `DeviceControlHandlers.SetVolume`, `GetSettings`, `SetSettings` → `WriteMhsStates`, `ReadMhsStates`, `WriteMhsStates`.
