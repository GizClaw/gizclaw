---
name: gizclaw-admin-firmware
version: 2.0.0
description: "Manage GizClaw declarative firmware channel configuration. Use for admin firmware list/get/create/put/delete and resource apply/show."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin Firmware

Use this skill to manage firmware metadata and the external HTTPS `.tar.zlib`
package configured for the stable, beta, and develop channels.

## When To Use

- User asks to list or inspect firmware configuration.
- User wants to configure a channel package URL, SHA-256, or compressed size.
- User wants to replace the complete desired three-channel configuration.

## How To Start

1. Determine the admin context and pass `--context <name>` when known.
2. Identify the target Firmware ID and desired channel metadata.
3. Put the full desired firmware state in a JSON file.
4. Inspect the current state before replacing it; put and apply are complete-state operations.

## Commands

```bash
<gizclaw> admin firmwares list --context <admin-context>
<gizclaw> admin firmwares get <firmware> --context <admin-context>
<gizclaw> admin firmwares create --file <firmware.json> --context <admin-context>
<gizclaw> admin firmwares put <firmware> --file <firmware.json> --context <admin-context>
<gizclaw> admin firmwares delete <firmware> --context <admin-context>
<gizclaw> admin apply --file <firmware-resource.json> --context <admin-context>
<gizclaw> admin show Firmware <firmware> --context <admin-context>
```

## Payload

```json
{
  "id": "devkit",
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
    "develop": {}
  }
}
```

## Behavior Notes

- The server stores configuration only; it does not upload, download, proxy, or
  unpack firmware packages.
- `url` must be an absolute HTTPS URL. `sha256` and `size` describe the exact
  complete `.tar.zlib` archive bytes that the peer downloads directly.
- Create, put, and apply require all three channel objects. There is no
  promotion, rollback, package movement, or implicit merge.
