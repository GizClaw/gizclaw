# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) defines the long-term memory boundary shared by Agent runtimes. It accepts raw observations and delegates fact extraction and persistence to a provider. Reads return provider-neutral facts and relevance scores. `genx.ModelContext` is composed before model invocation and is not the storage model of this package.

## Core boundary

`Store` exposes four operations:

- `Observe` submits raw text or ordered turns with role, speaker, and timestamps for fact extraction. Non-empty turn attributes are currently unsupported.
- `Recall` queries facts with natural language and typed filters.
- `Update` changes fact text and, when supported, patches attributes and checks a revision.
- `Delete` removes or retires a fact and, when supported, checks a revision.

Asynchronous providers return an operation in `ObserveResult`. Stores implementing `OperationWaiter` use the caller's `context.Context` to wait for completion. Constructors do not start background goroutines. Durable Flowcraft stores recover operation IDs from canonical episode facts after restart; extraction failures return a terminal failed operation.

`AppID`, `UserID`, `AgentID`, and `RunID` are business memory scopes. They do not replace process, credential, or remote-service tenant isolation. Mem0 Platform configurations require an API key and must set at least one of these scopes. Mem0 stores each configured entity as a separate memory layer; recall across multiple configured entities uses `OR`, not a hierarchical intersection.

## Providers

| Provider | Execution | Persistence and model configuration | Update limits |
| --- | --- | --- | --- |
| Flowcraft | In process | `dir` selects the Flowcraft workspace backend; model resource names are resolved through `FlowcraftModelLoader` | Append-only revisions with text, attribute, and revision checks; provider-owned fact fields cannot be patched as metadata |
| Mem0 | Remote HTTP | Platform uses `Authorization: Token`; self-hosted API keys use `X-API-Key`, and the deployment owns its model configuration | Text updates; unsupported filters, attribute patches, and conditional writes return `ErrUnsupported` |
| Volcengine AgentKit/Viking MEM0 | Volc control plane and Mem0 data plane | Requires the project data-plane endpoint; uses an explicit Mem0 API key or resolves one within a required memory project, optionally selecting an API key ID | Same behavior as the Mem0 data plane |

Without an extraction model, Flowcraft deterministically stores an observation as a `note`. Configured extraction, embedding, and rerank models require a model loader passed to `OpenFlowcraftStore` or `stores.NewWithStorageOptions`.

The Volc provider does not duplicate the fact CRUD protocol. It resolves the data-plane API key through signed `DescribeMemoryProjectDetail` and `DescribeAPIKeyDetail` calls, then reuses the Mem0 client. `volc_memory.mem0.endpoint` is mandatory so a Volc credential can never fall back to Mem0 Platform's public endpoint. `memory_project_id` is required for control-plane resolution; an optional `api_key_id` selects a key within that project.

## Server configuration

A logical memory store selects exactly one provider:

```yaml
stores:
  agent-memory:
    kind: memory
    flowcraft:
      dir: ${GIZCLAW_MEMORY_DIR}
      runtime_id: detective-game
      user_id: player-42
```

Mem0 Platform:

```yaml
stores:
  agent-memory:
    kind: memory
    mem0:
      endpoint: https://api.mem0.ai
      api_key: ${MEM0_API_KEY}
      flavor: platform
      app_id: detective-game
      user_id: player-42
      agent_id: narrator
```

Volcengine AgentKit/Viking MEM0:

```yaml
stores:
  agent-memory:
    kind: memory
    volc_memory:
      mem0:
        endpoint: ${VOLC_MEM0_ENDPOINT}
        user_id: player-42
      memory_project_id: ${VOLC_MEMORY_PROJECT_ID}
      region: cn-beijing
      access_key_id: ${VOLC_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_ACCESS_KEY_SECRET}
```

`cmd/internal/stores` expands environment variables and owns the Flowcraft lifecycle. HTTP clients, Volc credential resolvers, and Flowcraft model loaders are injected through `Options`.
The default server registry has no model loader and rejects non-empty Flowcraft model fields after environment expansion; an Agent runtime that supplies those fields must use the option-aware registry path.

Mem0 V3 search puts entity IDs only inside `filters`. Native entity, time, category, and memory-ID fields use their documented operators; `FilterNotIn` is encoded as `NOT` around `in`. Other provider-neutral fields address top-level custom metadata and support only equality and inequality. Operators without an exact remote equivalent return `ErrUnsupported`.

## Error semantics

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior and must not expose API keys, AK/SK credentials, or credential-bearing response bodies in facts, logs, or errors.

When a provider cannot preserve a filter, attribute patch, or conditional write, it returns `ErrUnsupported` instead of silently discarding the condition.
The same rule applies to per-turn attributes until a provider-neutral extraction-context mapping is defined.
