# GizClaw OTA

This document describes the OTA flow built on the firmware catalog and peer
firmware RPCs.

## Scope

OTA in GizClaw is split into two responsibilities:

- Firmware publishing and artifact download are handled by the firmware domain.
- Firmware application, version comparison, reboot, and progress reporting are
  device/runtime concerns.

The peer firmware RPC surface is intentionally small:

```text
server.firmware.list
server.firmware.get
server.firmware.download
```

There are no firmware-specific update report RPCs such as
`server.firmware.update.begin`, `server.firmware.update.progress`,
`server.firmware.update.complete`, or `server.firmware.update.fail`.
OTA status should be added to the shared runtime/status reporting path together
with other device state.

## Firmware Model

A firmware document represents one release line, for example `devkit` or
`h106`. It contains four channel slots:

```text
develop -> beta -> stable -> pending
```

Each slot may declare artifacts. An artifact is selected by `name`, for example
`main`, `assets`, or `bootloader`, and has a `kind` such as `app` or `data`.
Binary payload bytes are uploaded separately and stored in the configured
`firmware-assets` object store. The firmware document stores server-owned
metadata such as object path, SHA-256, size, content type, and upload time.

## Admin Flow

Admins own firmware publishing:

1. Create or update a firmware document.
2. Upload binary payloads for declared channel/artifact entries.
3. Release or rollback channel slots.
4. Assign firmware selection to a peer config.
5. Grant the peer `firmware.read` through ACL.

Example CLI flow:

```sh
gizclaw admin firmwares put devkit -f firmware.json --context admin
gizclaw admin firmwares upload-bin devkit --channel stable --bin main -f app.bin --context admin
gizclaw admin peers put-config <peer-public-key> --file peer-config.json --context admin
gizclaw admin firmwares release devkit --context admin
```

Peer config selects the firmware release line and channel:

```json
{
  "firmware": {
    "id": "devkit",
    "channel": "stable"
  }
}
```

## Device Flow

The device decides whether to update locally. The server does not provide a
separate `update.check` RPC.

1. Read the assigned firmware id and channel from peer config or local runtime
   context.
2. Call `server.firmware.get` to fetch the firmware document.
3. Compare local firmware version/build/artifact metadata with the assigned
   channel.
4. Pick the exact `artifact_name` needed by the device.
5. Call `server.firmware.download`.
6. Verify size and SHA-256 before applying the payload.
7. Apply the update using the device firmware updater.

Example peer CLI flow:

```sh
gizclaw connect firmware list --context device
gizclaw connect firmware get --firmware-id devkit --context device
gizclaw connect firmware download \
  --firmware-id devkit \
  --channel stable \
  --artifact-name main \
  --output app.bin \
  --context device
```

## Runtime Status

OTA status is not firmware metadata. It is runtime state.

Runtime/status reporting should eventually carry OTA state alongside other
device state such as network, cellular, battery, storage, and active runtime
data. A future runtime/status design should define:

- current OTA phase, if any;
- selected firmware id, channel, and artifact name;
- downloaded/applied byte counts;
- last error code and message;
- active image version and boot slot, if the device exposes them;
- history retention and TTL;
- admin UI display and filtering.

Until that shared reporting path exists, firmware RPCs should stay read and
download only.

## Storage

Server config needs both metadata and object stores:

```yaml
stores:
  firmwares:
    kind: keyvalue
    storage: main-kv
    prefix: firmwares

  firmware-assets:
    kind: objectstore
    storage: local-assets
    prefix: firmwares
```

The `firmwares` store holds firmware JSON metadata. The `firmware-assets` store
holds uploaded binary payloads.

## Access Control

Peer firmware access is controlled by ACL:

- admins manage firmware documents and uploads;
- peers need `firmware.read` on a firmware resource before `list`, `get`, or
  `download` can expose it;
- artifact downloads are authorized through the same firmware read permission.

This keeps firmware publishing under admin control while allowing devices to
consume only the release lines they are allowed to read.
