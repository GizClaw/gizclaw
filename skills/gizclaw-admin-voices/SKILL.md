---
name: gizclaw-admin-voices
version: 1.0.0
description: "Read GizClaw global voice catalog. Use for admin voices list/get and filtering by source, provider kind, or provider ID."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin Voices

Use this skill for global voice catalog reads.

## When To Use

- User asks to list or inspect voices.
- User asks to filter voices by source or provider metadata.
- User asks about synced voices after MiniMax tenant sync.

## How To Start

1. Determine the admin context and pass `--context <name>` when known.
2. For listing, apply filters if the user names source, provider kind, or provider ID.
3. Use `get <id>` when the user names a voice id.

## Commands

```bash
<gizclaw> admin voices list --context <admin-context>
<gizclaw> admin voices list --source <source> --context <admin-context>
<gizclaw> admin voices list --provider-kind <kind> --context <admin-context>
<gizclaw> admin voices list --provider-id <id> --context <admin-context>
<gizclaw> admin voices get <id> --context <admin-context>
```

## Behavior Notes

- This CLI resource surface is read-only: it exposes `list` and `get`.
- Use `../gizclaw-admin-resources/SKILL.md` for declarative `Voice`
  create, update, show, or delete workflows.
- `list` supports `--source`, `--provider-kind`, and `--provider-id` filters.
