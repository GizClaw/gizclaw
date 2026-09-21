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
