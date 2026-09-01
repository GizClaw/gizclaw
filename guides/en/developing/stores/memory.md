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

`Text` and `Turns` are raw extraction material; `Facts` are candidates already structured by the caller. A provider must preserve candidate text and supported attributes or return `ErrUnsupported`; it must not silently send candidates through model extraction again. Flowcraft, Mem0, and Volc all support direct Facts. Flowcraft maps `kind`, `subject`, `predicate`, `object`, and `entities` to native fields; Mem0 and Volc use `infer=false` direct import.

For an Observation containing direct Facts, a non-empty `Observation.ID` is an idempotency key within the complete `Scope`. Concurrent calls or retries with the same ID and canonical direct-Fact payload return the original logical Fact or durable operation. Changing Fact text, attributes, or `ObservedAt` returns `ErrConflict`. Adapters retain a payload digest in native records and reconcile before submission, so retrying after a provider accepted a request but lost its response does not create a second logical Fact. Returned `Fact.Sources` retain the `ObservationID`; provider-owned metadata is not exposed as business attributes. Provider-native deduplication for model extraction is outside this direct-Fact guarantee.

`UpdateRequest`, `DeleteRequest`, and `OperationRequest` resubmit the caller's `Scope` together with an opaque fact, revision, or operation locator returned by the Store. A locator is not an authorization source: before a mutation or asynchronous completion is accepted, the adapter verifies that the requested Scope matches the locator and provider record. A raw provider ID cannot bypass an App boundary.

Asynchronous `Observe` calls return an operation. Stores implementing `OperationWaiter` wait using the caller's `context.Context`; constructors do not start background goroutines. The Flowcraft constructor neither enumerates durable scopes nor reads canonical facts to warm an operation cache. After the adapter is reconstructed with the same injected stores, `Wait()` decodes the locator, validates the caller's complete `Scope`, and recovers the durable operation from only the locator's scope; a mismatched scope returns `ErrInvalidInput` before the temporal store is read.

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

Mem0 is constructed with one `mem0.Config`. `FlavorPlatform` uses `Authorization: Token` and maps every selected dimension to the matching `app_id`, `user_id`, `agent_id`, or `run_id`. Mem0 OSS does not expose `app_id`, so `FlavorSelfHosted` encodes the complete four-dimensional Scope into one reserved native `user_id`; it uses `X-API-Key` when a key is supplied. This keeps Workspace App isolation exact without overwriting the caller's logical User, Agent, or Run dimensions. Update and delete retrieve the provider record and verify its complete encoded scope before performing the ID mutation. Direct import currently accepts one Fact with a non-empty Observation ID per call; multiple direct candidates return `ErrUnsupported` instead of silently merging attributes.

Volcengine AgentKit/Viking MEM0 is constructed with one `volc.Config`. It accepts either an explicit Mem0 data-plane key or a credential resolver. The adapter explicitly selects Volc's v1 add/search routes, extracts one authoritative job ID from `results`, and makes `Wait` poll `/v1/job/{id}/`. If a successful job omits facts, it lists only the same scope and selects records owned by the observation ID. The Volc v1 service requires `user_id`, so an App-, Agent-, or Run-only logical scope receives a reserved encoded transport user while retaining every original native field; returned records are decoded and checked against the unchanged logical scope. Generic Mem0 Platform continues to use v3 add/search, its top-level event ID, and `/v1/event/{id}/`. Endpoint hostnames are never used to infer the protocol. A Volc data-plane endpoint is mandatory.

An Eino `memory_observe` node assigns each Graph-authored direct Fact a stable observation identity derived from the current turn and Graph node. A following `memory_recall` node in the same Workspace therefore reads the Fact through the same complete `Scope.AppID`, including with `volc_mem0`. Volc search results must have a native Fact ID and a compatible encoded scope; provider routing metadata such as project and strategy bookkeeping is not returned as business attributes.

## MemoryLayout, RuntimeProfile, and Workflow

Memory is no longer a `stores.kind: memory` Server Config entry. Portable policy, deployment connection, and Graph consumption belong to three separate resource surfaces:

- The Admin `MemoryLayout` declares provider policy for Flowcraft, Mem0, and `volc_mem0` together. It contains no endpoint, API key, DSN, or directory.
- `RuntimeProfile.resources.memories.<alias>` selects the Layout, concrete driver, and a strictly typed connection. Endpoint, API key, project ID, DSN, or directory belongs directly to that RuntimeProfile connection and does not reference a Credential resource.
- A Workflow top-level `memory` field references only a RuntimeProfile alias. Graph `memory_recall` and `memory_observe` nodes own timing, query source, output destination, and turn/state-to-fact construction; those mappings do not belong to MemoryLayout.

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata:
  name: pet-memory
spec:
  flowcraft:
    extraction:
      enabled: true
      model: pet-care.extract
      mode: two_pass
    embedding:
      model: pet-care.embedding
    lanes:
    - name: owner-profile
      kind: preference
    write:
      mode: sync
      tier: general
  mem0:
    custom_instructions: Extract durable pet and owner facts.
  volc_mem0:
    strategies:
    - name: owner-profile
      type: user_preference
      custom_instructions: Extract durable pet and owner facts.
```

All three provider blocks are required. Flowcraft extraction, embedding, and rerank models are RuntimeProfile model aliases; they accept the same 1-63-byte dot-separated lowercase kebab-case grammar as RuntimeProfile bindings. Each complete alias is an opaque flat key resolved exactly, without prefix, segment, or fallback lookup. They are resolved only when the binding selects `driver: flowcraft`. `extraction.enabled` defaults to `true`; setting it to `false` disables model extraction while Graph-authored direct Facts remain writable.

```yaml
spec:
  resources:
    memories:
      pet-memory:
        layout_id: pet-memory
        driver: flowcraft
        connection:
          type: flowcraft_redis8
          url: redis://redis:6379/0
```

The Server opens that physical backend once per binding and gives every Workspace generation an independently closable logical Store. Logical Store construction for different Workspaces proceeds concurrently when the published Flowcraft projection signature is unchanged; it is not part of the binding-registry map critical section. A policy change still has one serialized projection-rebuild owner. Other constructors continue only after the complete replacement is atomically published. Normal final-lease cleanup closes the physical backend after the last logical lease and in-flight constructor have left; explicit Registry shutdown detaches the binding, rejects late constructor results, drains those constructors, and then closes the physical backend.

The valid Flowcraft connections are `flowcraft_postgresql` (`dsn`) and `flowcraft_redis8` (`url`, optional `tls_ca_file`). `flowcraft_redis8` requires Redis 8.4 or newer with Redis Search and keeps canonical Facts, Evidence, the Async Semantic Queue, the Side-effect Outbox, and text/vector retrieval in one Redis namespace; it does not fall back to Redis 7 or Redis 8.0/8.2. Retrieval uses Redis-native BM25, HNSW KNN, structured metadata filtering, top-K limiting, and `FT.HYBRID` RRF fusion. `rediss://` reuses Storage TLS verification and `tls_ca_file` can add a trusted CA. Flowcraft 0.1.7 does not expose a Graph store injection port, so this connection rejects `graph_enabled` instead of silently using a non-durable process-local Graph. Driver and connection type must match. Unknown fields, missing keys, and invalid endpoints are rejected when a RuntimeProfile is written or resolved.

Flowcraft 0.1.7 defines `(runtime_id, user_id)` as the canonical hard partition. `agent_id` is soft-isolation metadata and is intentionally excluded from `ScopeEnumerator`; enumerating that hard scope still reads facts written with every AgentID in the partition instead of fragmenting cross-agent recall.

For Mem0 and Volc, the Project ID records the deployment/control-plane identity paired with the selected data-plane API key. Runtime fact requests authenticate with that key; they do not send a separate Project ID field.

```yaml
spec:
  driver: flowcraft
  memory: pet-memory
  flowcraft:
    graph:
      name: companion
      entry: recall-memory
      nodes:
      - id: recall-memory
        type: memory_recall
        config:
          query: {text_from: input}
          output: memory_context
          top_k: 5
      - id: answer
        type: llm
        publish: true
        config:
          model: chat
          system_prompt: "${board.memory_context}"
      - id: observe-turn
        type: memory_observe
        config:
          observations:
          - turns_from: conversation
          wait_for_completion: false
      edges:
      - {from: recall-memory, to: answer}
      - {from: answer, to: observe-turn}
      - {from: observe-turn, to: __end__}
```

All streams for one Workspace share one Agent generation. Stable data visibility requires the same Workspace AppID, memory driver, and physical connection selected by the same RuntimeProfile memory binding. Changing extraction, recall, write, prompt, `top_k`, or mode does not change canonical data. When a Flowcraft derived-index policy changes, a staging index is rebuilt from canonical facts and published atomically; a failed rebuild never publishes a partial or mixed index. Changing driver or binding may select another physical source and does not migrate or delete data automatically. Switching back can access the original data if the original connection still retains it.

## Ownership and errors

Provider adapters do not close injected dependencies. The composition root that constructs a workspace, index, HTTP client, or credential dependency owns it and closes resources in reverse construction order.

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior. If a provider cannot preserve a filter, attribute patch, or conditional-write semantic, it returns `ErrUnsupported` rather than discarding the condition. Errors must not expose API keys, access-key credentials, or credential-bearing response bodies.

Physical Memory Store construction is coordinated per complete binding key: callers for the same key share one backend, while unrelated bindings construct independently. Direct Mem0 Fact idempotency is coordinated by complete canonical Scope plus observation ID, so a slow provider request does not stop unrelated scopes or observations; same-key retries still reconcile or return `ErrConflict`.
