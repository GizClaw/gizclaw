# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) is the provider-neutral long-term memory boundary used by Agent runtimes. Provider adapters live in the `flowcraft`, `mem0`, and `volc` subpackages.

## Contract

Every operation carries a structured `Scope`. `AppID`, `UserID`, `AgentID`, and `RunID` are four independent optional routing dimensions. The common contract does not define an App→User→Agent→Run hierarchy and does not interpret `RunID` as a universal Conversation. An empty field means that dimension was not selected; it is not a wildcard, inherited value, or global-visibility marker.

```go
result, err := store.Observe(ctx, memory.Observation{
	Scope: memory.Scope{
		AppID:  "game",
		UserID: "player-42",
	},
	Turns: turns,
	Facts: []memory.FactCandidate{{
		Text: "story_progress: current_beat=origin",
		Attributes: map[string]any{"kind": "state"},
	}},
})

recalled, err := store.Recall(ctx, memory.Query{
	Scope: memory.Scope{
		AppID:  "game",
		UserID: "player-42",
	},
	Text:  "What does the player prefer?",
	Limit: 10,
})
```

Adapters preserve only dimensions and combinations that their native provider can represent exactly; otherwise they return `ErrUnsupported`:

| Common field | Flowcraft Recall | Mem0 / Volc Memory |
| --- | --- | --- |
| `AppID` | `RuntimeID` | `app_id` |
| `UserID` | `UserID` | `user_id` |
| `AgentID` | `AgentID` | `agent_id` |
| `RunID` | unsupported | `run_id` |

Flowcraft requires a non-empty `AppID`, allows an empty `UserID` for runtime-global Memory, and preserves an optional `AgentID`. Mem0/Volc supports app-only, user-only, agent-only, and run-only scopes as well as combinations of those independent dimensions. The adapter sends every selected dimension and uses an `AND` filter for recall and mutation verification.

`Text` and `Turns` are raw extraction material; `Facts` are candidates already structured by the caller. A provider must preserve candidate text and supported attributes or return `ErrUnsupported`; it must not silently send candidates through model extraction again. The Flowcraft adapter supports direct ingestion and maps `kind`, `subject`, `predicate`, `object`, and `entities` to native fact fields.

`UpdateRequest`, `DeleteRequest`, and `OperationRequest` resubmit the caller's `Scope` together with an opaque fact, revision, or operation locator returned by the Store. A locator is not an authorization source: before a mutation or asynchronous completion is accepted, the adapter verifies that the requested Scope matches the locator and provider record. A raw provider ID cannot bypass an App boundary.

Asynchronous `Observe` calls return an operation. Stores implementing `OperationWaiter` wait using the caller's `context.Context`; constructors do not start background goroutines. Flowcraft can recover durable operation locators after the adapter is reconstructed with the same injected stores.

`memory.BindApp(store, appID)` returns a borrowed Store view. It only fills or verifies `Scope.AppID`; it never generates, clears, concatenates, hashes, or rewrites caller-supplied `UserID`, `AgentID`, or `RunID`. A conflicting AppID returns `ErrInvalidInput`. The view does not own or close the underlying Store, and it exposes `OperationWaiter`, `AsyncOperationProcessor`, and `StatisticsProvider` only when the underlying Store provides the same capability.

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

Mem0 is constructed with one `mem0.Config`. `FlavorPlatform` uses `Authorization: Token` and maps every selected dimension to the matching `app_id`, `user_id`, `agent_id`, or `run_id`. Mem0 OSS does not expose `app_id`, so `FlavorSelfHosted` encodes the complete four-dimensional Scope into one reserved native `user_id`; it uses `X-API-Key` when a key is supplied. This keeps Workspace App isolation exact without overwriting the caller's logical User, Agent, or Run dimensions. Update and delete retrieve the provider record and verify its complete encoded scope before performing the ID mutation.

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

Current Flowcraft fact and operation locators contain the complete App/User/Agent scope. Legacy `flowcraft:v1` locators and development data are incompatible and must be cleared and recreated; there is no compatibility decoder, dual read, or background migration.

## Ownership and errors

Provider adapters do not close injected dependencies. The composition root that constructs a workspace, index, HTTP client, or credential dependency owns it and closes resources in reverse construction order.

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior. If a provider cannot preserve a filter, attribute patch, or conditional-write semantic, it returns `ErrUnsupported` rather than discarding the condition. Errors must not expose API keys, access-key credentials, or credential-bearing response bodies.
