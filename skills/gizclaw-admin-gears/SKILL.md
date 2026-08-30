---
name: gizclaw-admin-gears
version: 1.0.0
description: "Manage GizClaw registered devices/gears. Use for admin gears list/get/find-sn/resolve-imei/approve/block/delete/refresh/info/config/runtime/ota/list-by-* and gear config changes."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin Gears

Use this skill for device registration records, identifiers, gear snapshots,
gear configuration, runtime state, OTA summaries, and indexed lookup.

## When To Use

- User asks to list or inspect registered devices.
- User asks to approve, block, delete, or refresh a gear.
- User has a serial number, IMEI parts, label, certification, firmware depot, or public key and wants lookup.
- User asks to read or update gear configuration.

## How To Start

1. Determine the admin context. Use `--context <name>` if provided; otherwise check current context.
2. Identify the target: public key, serial number, IMEI TAC+serial, label, certification, or firmware policy.
3. For mutating commands, confirm the target comes from the user or prior command output.
4. For full config replacement, write a JSON file and use `put-config --file`.

## Commands

```bash
<gizclaw> admin gears list --context <admin-context>
<gizclaw> admin gears get <pubkey> --context <admin-context>
<gizclaw> admin gears find-sn <sn> --context <admin-context>
<gizclaw> admin gears resolve-imei <tac> <serial> --context <admin-context>
<gizclaw> admin gears approve <pubkey> <role> --context <admin-context>
<gizclaw> admin gears block <pubkey> --context <admin-context>
<gizclaw> admin gears delete <pubkey> --context <admin-context>
<gizclaw> admin gears refresh <pubkey> --context <admin-context>
<gizclaw> admin gears info <pubkey> --context <admin-context>
<gizclaw> admin gears config <pubkey> --context <admin-context>
<gizclaw> admin gears runtime <pubkey> --context <admin-context>
<gizclaw> admin gears ota <pubkey> --context <admin-context>
<gizclaw> admin gears list-by-label <key> <value> --context <admin-context>
<gizclaw> admin gears list-by-certification <type> <authority> <id> --context <admin-context>
<gizclaw> admin gears list-by-firmware <depot> <channel> --context <admin-context>
```

## Config Updates

Use `set-firmware-channel` for a firmware-channel-only change:

```bash
<gizclaw> admin gears set-firmware-channel <pubkey> <channel> --context <admin-context>
```

Use `put-config --file` for complete gear configuration replacement:

```bash
<gizclaw> admin gears put-config <pubkey> --file <config.json> --context <admin-context>
```

Example config file:

```json
{
  "firmware": {
    "channel": "stable"
  },
  "certifications": [
    {
      "type": "certification",
      "authority": "ce",
      "id": "ce-001"
    }
  ]
}
```

## Behavior Notes

- `approve` changes a pending registration to the requested role.
- `block` disables a gear.
- `delete` resets/removes the stored gear registration record.
- `refresh` asks the server to pull snapshots from the device-side reverse API when available.
- `find-sn` returns every gear reporting the supplied SN; an SN is not treated as a unique server identity.
- `info`, `config`, `runtime`, and `ota` are read-only JSON snapshots.
- Use `../gizclaw-admin-resources/SKILL.md` for declarative `GearConfig`
  apply/show workflows when the user is working with Resource JSON envelopes.
- `GearConfig` cannot be deleted independently through the generic resource API.
