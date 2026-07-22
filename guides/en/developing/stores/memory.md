# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) is the provider-neutral long-term memory boundary used by Agent runtimes. Provider adapters live in the `flowcraft`, `mem0`, and `volc` subpackages.

## Contract

`Observation.Scope` and `Query.Scope` are opaque product-owned namespaces. Callers decide which product context shares a memory namespace, but they do not provide provider-native fields such as runtime, user, agent, run, project, or worker IDs. Each adapter privately maps the opaque scope to its backend protocol.

```go
result, err := store.Observe(ctx, memory.Observation{
	Scope: memory.Scope("player:42"),
	Turns: turns,
	Facts: []memory.FactCandidate{{
		Text: "story_progress: current_beat=origin",
		Attributes: map[string]any{"kind": "state"},
	}},
})

recalled, err := store.Recall(ctx, memory.Query{
	Scope: memory.Scope("player:42"),
	Text:  "What does the player prefer?",
	Limit: 10,
})
```

`Text` and `Turns` are raw extraction material; `Facts` are candidates already structured by the caller. A provider must preserve candidate text and supported attributes or return `ErrUnsupported`; it must not silently send candidates through model extraction again. The Flowcraft adapter supports direct ingestion and maps `kind`, `subject`, `predicate`, `object`, and `entities` to native fact fields.

`Update` and `Delete` take only the opaque fact ID previously returned by the store, plus an optional opaque revision. `Wait` takes only the opaque operation ID. Callers must not resubmit a scope for identity-based operations; adapters encode any routing state they need in those opaque locators.

Asynchronous `Observe` calls return an operation. Stores implementing `OperationWaiter` wait using the caller's `context.Context`; constructors do not start background goroutines. Flowcraft can recover durable operation locators after the adapter is reconstructed with the same injected stores.

## Provider construction

Provider packages accept in-memory runtime dependencies only. They do not decode YAML, expand environment variables, open configuration files, or choose product identity.

Flowcraft is constructed with one `flowcraft.Config`. The config can inject a `ModelLoader`, retrieval index, temporal store, evidence store, async queue, and side-effect outbox. Injected dependencies remain caller-owned. If a dependency is omitted, the adapter uses Flowcraft's in-memory implementation.

```go
store, err := flowcraft.New(ctx, flowcraft.Config{
	Loader:         loader,
	Extraction:     flowcraft.ExtractionConfig{Model: "extractor"},
	Embedding:      flowcraft.EmbeddingConfig{Model: "embedding"},
	RetrievalIndex: index,
	TemporalStore:  temporal,
})
```

Mem0 is constructed with one `mem0.Config`. `FlavorPlatform` uses `Authorization: Token`; `FlavorSelfHosted` uses `X-API-Key` when a key is supplied. The adapter maps an operation scope to Mem0's native `user_id` internally. Update and delete use the returned memory ID directly.

Volcengine AgentKit/Viking MEM0 is constructed with one `volc.Config`. It accepts either an explicit Mem0 data-plane key or a credential resolver. The adapter resolves credentials and delegates fact operations to the Mem0 adapter; a data-plane endpoint is mandatory.

## Composition and YAML

`cmd/internal/stores` is the composition root. It owns serializable YAML DTOs, environment expansion, workspace/index construction, model-loader injection, credential resolution, and lifecycle management. A Flowcraft `dir` belongs to this layer: the command creates the Flowcraft workspace and BBH retrieval index, then injects their interfaces into the adapter.

```yaml
stores:
  agent-memory:
    kind: memory
    flowcraft:
      dir: ${GIZCLAW_MEMORY_DIR}
      extraction_model: memory-extractor
      embedding_model: text-embedding
      extraction_mode: single_pass
      graph_enabled: true
      async:
        enabled: true
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
```

Volcengine AgentKit/Viking MEM0:

```yaml
stores:
  agent-memory:
    kind: memory
    volc_memory:
      mem0:
        endpoint: ${VOLC_MEM0_ENDPOINT}
      memory_project_id: ${VOLC_MEMORY_PROJECT_ID}
      region: cn-beijing
      access_key_id: ${VOLC_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_ACCESS_KEY_SECRET}
```

A logical memory store selects exactly one provider. Unknown YAML fields are rejected. Scope and backend-native routing fields are not valid server configuration.

## Ownership and errors

Provider adapters do not close injected dependencies. The composition root that constructs a workspace, index, HTTP client, or credential dependency owns it and closes resources in reverse construction order.

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior. If a provider cannot preserve a filter, attribute patch, or conditional-write semantic, it returns `ErrUnsupported` rather than discarding the condition. Errors must not expose API keys, access-key credentials, or credential-bearing response bodies.
