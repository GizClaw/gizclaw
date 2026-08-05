---
name: gizclaw-admin-firmware
version: 2.0.0
description: "Manage GizClaw firmware channel configuration, releases, and rollbacks. Use for admin firmware list/get/create/put/delete/release/rollback."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin Firmware

Use this skill to manage firmware metadata and the external HTTPS `.tar.zlib`
package configured for each release channel.

## When To Use

- User asks to list or inspect firmware configuration.
- User wants to configure a channel package URL, SHA-256, or compressed size.
- User wants to promote or roll back firmware channels.

## How To Start

1. Determine the admin context and pass `--context <name>` when known.
2. Identify the target firmware name and channel.
3. Put the full desired firmware state in a JSON file.
4. Before release or rollback, inspect the current stable, beta, and pending slots.

## Commands

```bash
<gizclaw> admin firmwares list --context <admin-context>
<gizclaw> admin firmwares get <firmware> --context <admin-context>
<gizclaw> admin firmwares create --file <firmware.json> --context <admin-context>
<gizclaw> admin firmwares put <firmware> --file <firmware.json> --context <admin-context>
<gizclaw> admin firmwares delete <firmware> --context <admin-context>
<gizclaw> admin firmwares release <firmware> --context <admin-context>
<gizclaw> admin firmwares rollback <firmware> --context <admin-context>
```

## Payload

```json
{
  "name": "devkit",
  "slots": {
    "stable": {
      "description": "stable channel",
      "package": {
        "url": "https://downloads.example.com/devkit/stable.tar.zlib",
        "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
        "size": 1048576
      }
    },
    "beta": {},
    "develop": {},
    "pending": {}
  }
}
```

## Behavior Notes

- The server stores configuration only; it does not upload, download, proxy, or
  unpack firmware packages.
- `url` must be an absolute HTTPS URL. `sha256` and `size` describe the exact
  complete `.tar.zlib` archive bytes that the peer downloads directly.
- `release` moves `pending -> stable`, `stable -> beta`, and `beta -> develop`,
  then clears `pending`.
- `rollback` moves `stable -> pending`, `beta -> stable`, and
  `develop -> beta`, then clears `develop`.
