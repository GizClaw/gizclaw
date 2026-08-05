---
name: gizclaw-admin-resources
version: 1.0.0
description: "Manage GizClaw declarative admin resources. Use for admin apply/show/delete with Resource or ResourceList JSON files."
metadata:
  requires:
    bins: ["gizclaw"]
---

# GizClaw Admin Resources

Use this skill for declarative admin resource workflows backed by the generic
resource API.

## When To Use

- User asks to create, update, inspect, or delete an identified admin resource using
  `apiVersion`, `kind`, `metadata`, and `spec`.
- User provides or requests a Resource/ResourceList JSON file.
- User wants idempotent desired-state changes across credentials, MiniMax
  tenants, voices, workspace templates, workspaces, or gear config.
- User asks for `admin apply`, `admin show`, or `admin delete`.

## How To Start

1. Determine the admin context and pass `--context <name>` when known.
2. Identify the resource `kind` and caller-supplied `metadata.id`.
3. For create/update, write a Resource or ResourceList JSON file and use
   `apply -f`.
4. For inspection, use `show <kind> <id>` with a concrete resource kind.
5. For deletion, use `delete <kind> <id>` only when the kind supports delete.

## Commands

```bash
<gizclaw> admin apply -f <resource.json> --context <admin-context>
<gizclaw> admin apply -f - --context <admin-context>
<gizclaw> admin show <kind> <id> --context <admin-context>
<gizclaw> admin delete <kind> <id> --context <admin-context>
```

## Resource Kinds

Concrete kinds:

- `Credential`
- `Firmware`
- `Contact`
- `Friend`
- `FriendGroup`
- `FriendGroupInviteToken`
- `FriendGroupMember`
- `Model`
- `DashScopeTenant`
- `DeepSeekTenant`
- `GeminiTenant`
- `MiniMaxTenant`
- `OpenAITenant`
- `VolcTenant`
- `Voice`
- `Tool`
- `Workflow`
- `Workspace`
- `PetDef`
- `BadgeDef`
- `GameDef`
- `MemoryLayout`
- `RuntimeProfile`
- `RegistrationToken`

Container kind:

- `ResourceList`

`ResourceList` is a batch envelope without `metadata`; it can be applied but
cannot be shown or deleted by ID.

## Examples

Credential resource:

```json
{
  "apiVersion": "gizclaw.admin/v1alpha1",
  "kind": "Credential",
  "metadata": {
    "id": "minimax-main"
  },
  "spec": {
    "provider": "minimax",
    "method": "api_key",
    "body": {
      "api_key": "secret"
    }
  }
}
```

Show or delete the same resource by `metadata.id`:

```bash
<gizclaw> admin show Credential minimax-main --context <admin-context>
<gizclaw> admin delete Credential minimax-main --context <admin-context>
```

ResourceList:

```json
{
  "apiVersion": "gizclaw.admin/v1alpha1",
  "kind": "ResourceList",
  "spec": {
    "items": [
      {
        "apiVersion": "gizclaw.admin/v1alpha1",
        "kind": "Credential",
        "metadata": {
          "id": "minimax-main"
        },
        "spec": {
          "provider": "minimax",
          "method": "api_key",
          "body": {
            "api_key": "secret"
          }
        }
      }
    ]
  }
}
```

## Behavior Notes

- `apply` creates, updates, or leaves resources unchanged and returns an
  `action` such as `created`, `updated`, or `unchanged`.
- Every concrete resource ID is supplied by the caller, persisted unchanged,
  and used by all downstream Admin references. The Server does not translate
  legacy names into IDs.
- `show` returns the stored declarative resource state.
- `delete` returns the deleted declarative resource state.
- Prefer `apply` for structured admin writes instead of older per-resource
  write flows when the user is working with declarative resources.
- Credential `body` values are returned by the admin API. Do not mask them
  unless requested.
