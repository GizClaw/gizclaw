# services/device

`pkgs/gizclaw/services/device` saves server resources owned by the device domain. Currently, the directory only has `firmware/`, which is responsible for the Firmware catalog and OTA channel configuration.

## Directory structure

```text
services/device/
└── firmware/    # Firmware metadata and external channel packages
```

## firmware

`firmware` owns:

- Firmware catalog and channel metadata.
- Validation and persistence of each channel's HTTPS `.tar.zlib` URL, SHA-256, and archive size.
- Complete replacement and strict validation of stable, beta, and develop slots.

It does not own device connections, peer registration, runtime status, or telemetry. What transport is the device connected through, whether it is currently online, and what status is reported, which belongs to the root peer connection and `services/runtime`.

## Dependencies and boundaries

```mermaid
flowchart LR
    GizClaw["pkgs/gizclaw<br/>Admin surface"] --> Firmware["services/device/firmware"]
    Firmware --> SQL["SQL catalog"]
```

Should be placed at `services/device/firmware`:

- Domain rules for Firmware and channels.
- Declarative external package metadata for stable, beta, and develop.
- Validation of firmware configuration as untrusted input.

Shouldn't be placed here:

- WebRTC connection, device signaling or telemetry transport.
- Peer identity, RegistrationToken, or generic resource ownership.
- Board-specific flash, bootloader or firmware implementation.
- Package download, proxy, unpacking, or binary storage.
- Creation of CLI storage backend and filesystem root.

When adding device domain services in the future, you should first confirm whether it has independent resources and life cycle before deciding to add `services/device/<service>`. Do not put all device-related logic into `firmware/`.

The Firmware catalog uses the `firmwares` business table. ID is the primary key; description and creation/update timestamps have separate columns, while channel configuration remains JSON. Server startup initializes the schema using the configured shared SQL pool; requests never execute DDL. Lists use ID range queries and SQL limits, and updates/deletes use `RETURNING` without KV enumeration.

Package `version` is a required SemVer 2.0.0 string of at most 128 ASCII characters. Release, prerelease and build metadata are accepted; a leading `v`, surrounding whitespace and illegal numeric leading zeros are rejected. An empty channel may omit the entire package. Create, put and resource apply validate versions before writing; rejected replacements preserve the stored configuration. Channels may select lower versions for rollback. Versions support display and SemVer ordering; build metadata does not affect precedence, and SHA-256 still identifies exact archive bytes. The service does not infer versions from URLs or descriptions or backfill stored records; reads of records with missing or invalid versions return an internal error instead of a schema-invalid package. Operators repair these records with the complete configuration through direct Admin PUT; resource apply first reads the old record and cannot perform this repair. Delete rolls back if the returned record fails validation, preserving the original record.
