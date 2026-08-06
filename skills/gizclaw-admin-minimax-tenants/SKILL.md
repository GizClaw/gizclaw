---
name: gizclaw-admin-minimax-tenants
version: 1.0.0
description: "Read GizClaw MiniMax tenants. Use for admin minimax-tenants list/get."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin MiniMax Tenants

Use this skill for MiniMax tenant reads.

## When To Use

- User asks to list or inspect MiniMax tenant app/group/credential information.
- User asks which MiniMax tenants exist.
- User asks to verify a tenant's configured credential reference.

## How To Start

1. Determine the admin context and pass `--context <name>` when known.
2. Use `list` first when the tenant ID is unknown.
3. Use `get <id>` for one tenant.
4. Use `admin credentials get <credential_id>` if the user needs to inspect the referenced credential.

## Commands

```bash
<gizclaw> admin minimax-tenants list --context <admin-context>
<gizclaw> admin minimax-tenants get <id> --context <admin-context>
```

## Behavior Notes

- This CLI resource surface is read-only: it exposes `list` and `get`.
- Use `../gizclaw-admin-resources/SKILL.md` for declarative
  `MiniMaxTenant` create, update, show, or delete workflows.
- Voice synchronization remains outside the generic resource apply path.
- `credential_id` is the exact caller-supplied ID of an existing Credential.
