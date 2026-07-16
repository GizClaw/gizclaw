# pkgs/store/kv

`pkgs/store/kv` Defines GizClaw’s general ordered key-value abstraction. Key uses string segments to express hierarchical paths, and Store provides get, set, delete, prefix list and ordered traversal capabilities.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/store/kv)

## Core structure and implementation

| Symbol | Function |
| --- | --- |
| `Key` / `Entry` | Express segmentation keys and read results. |
| `Store` | Define CRUD, prefix listing and iterator contract. |
| `Options` | Configure store behaviors such as key separator. |
| `Memory` / `NewMemory` | In-process ordered store. |
| `Badger` / `NewBadger` | Badger-backed persistent implementation. |
| `Prefixed` | Add a fixed key namespace to the existing Store. |
| `ListAfter` | Read in pages after the specified key under prefix. |

## Ownership Boundary

`kv` Only defines the byte payload and hierarchical key semantics, and does not explain the field type of the payload. Serialization, resource validation, secondary index, and cross-record consistency are the responsibility of the domain service using it. Callers should use stable prefixes to isolate data and cannot rely on the internal key layout of other fields.
