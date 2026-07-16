# pkgs/gizclaw/customid

`pkgs/gizclaw/customid` Saves custom resource ID rules used by multiple GizClaw domains, including construction, formatting, and parsing boundaries for stable IDs and compound IDs.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw/customid)

## Directory location

```text
pkgs/gizclaw/customid/
└── GizClaw cross-service resource ID rules
```

The value of this package is to allow different public surfaces and services to use consistent IDs for the same type of product resources, instead of providing a unified helper for all database keys.

## Ownership Boundary

Should be placed at `customid`:

- Resource ID format recognized by multiple GizClaw services or API surfaces.
- Stable combination and parsing rules for each part in Compound ID.
- Format validation of Public/resource ID.

Should not be placed in `customid`:

- Database row ID used internally by a single domain.
- KV store prefix, object storage path or temporary cache key.
- Giznet public key and transport identity.
- Common base libraries such as UUID, hash or encoding.
- New string concatenation helper for hiding unstable resource models.

`customid` should only be entered if the ID is a GizClaw product contract across packages; realm-private IDs should stay in their owner package.
