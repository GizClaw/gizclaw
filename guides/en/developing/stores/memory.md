# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) is the provider-neutral long-term memory boundary used by Agent runtimes. Provider adapters live in the `flowcraft`, `mem0`, and `volc` subpackages.

MemoryLayout exposes a separate optional `mem0_self_hosted` policy containing
`scope: workspace|peer` and `custom_instructions`. A `mem0_self_hosted` connection
requires this block explicitly; `{}` uses Workspace scope and the service default
instructions. Construction, reload, scope routing, recall, observation and cleanup
select it by connection type. Cloud `mem0` categories, multilingual and decay
options remain independent. Self-hosted model/embedding/pgvector configuration
belongs to the service deployment. Existing Cloud-only layouts remain valid.

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

`ScopePurger` is the optional capability that irreversibly removes one scope. The purge set is exactly the Facts that `Recall` with the same `Scope` can return, plus provider-owned records that could later materialize into that set (pending extraction jobs, derived indexes, and provenance markers). `PurgeScope` is idempotent; when native bulk deletion cannot express that set exactly it returns `ErrUnsupported` instead of deleting more or less. `ScopeEmpty` reports whether any Fact of the purge set remains. Callers use `memory.PurgeScope` and `memory.ScopeEmpty`, which unwrap `BindApp` views and apply their AppID binding first, and return `ErrUnsupported` when the underlying Store lacks the capability.

| Provider | Purge | Verification |
| --- | --- | --- |
| Flowcraft | `ForgetAll(ForgetHard)` removes the `(runtime_id, user_id)` hard partition's canonical facts, markers, every projection, evidence, and the partition's async semantic and side-effect jobs. `AgentID` is soft metadata inside a partition, so a scope that selects `AgentID` returns `ErrUnsupported` | the canonical temporal store holds no revision in the partition |
| Mem0 Platform | `DELETE /v1/memories/` with the selected dimensions ANDed; a dimension whose value is `*` is a provider wildcard and returns `ErrInvalidInput` | `POST /v3/memories/` lists the same filter |
| Mem0 self-hosted | `DELETE /memories?user_id=<complete scope encoding>` | `GET /memories` with the same `user_id` |
| Volc | `DELETE /v1/memories/?user_id=<reserved scope user>`. Volc bulk delete accepts only `user_id`, `agent_id`, and `run_id` and cannot narrow a caller-selected `UserID` to one App, so such a scope returns `ErrUnsupported` | `GET /v1/memories/` with the dimensions used on write |

Mem0 Platform deletes asynchronously, and a Volc `async_mode` add job can materialize after a purge. Callers that need a durable guarantee purge and verify repeatedly until `ScopeEmpty` reports true. Flowcraft `NewMaintenance` builds a Store for purge and verification over caller-owned persistent dependencies: it loads no model and accepts an async queue without an extraction model so the purge also cancels queued jobs; its `Observe` and `ProcessAsync` return `ErrUnsupported`.

## Provider construction

Provider packages accept in-memory runtime dependencies only. They do not decode YAML, expand environment variables, open configuration files, or choose product identity.

Flowcraft is constructed with one `flowcraft.Config`. The config can inject a `ModelLoader`, retrieval index, temporal store, evidence store, async queue, and side-effect outbox. Injected dependencies remain caller-owned. If a dependency is omitted, the adapter uses Flowcraft's in-memory implementation. Before a side-effect outbox job reaches the injected outbox, the adapter assigns it a scope-qualified identity (`<scope canonical key>|<request ID>|<kind>`) only when the job carries no ID and has a non-empty request ID; a caller-supplied ID and a job without a request ID pass through unchanged. Flowcraft's own Save batches always arrive without an ID, so concurrent Saves in different scopes that share one outbox never dedupe each other's projection, embedding, or evolution jobs, while replay of one scope's batch stays idempotent.

```go
store, err := flowcraft.New(ctx, flowcraft.Config{
	Loader:         loader,
	Extraction:     flowcraft.ExtractionConfig{Model: "extractor"},
	Embedding:      flowcraft.EmbeddingConfig{Model: "embedding"},
	RetrievalIndex: index,
	TemporalStore:  temporal,
})
```

Mem0 is constructed with one `mem0.Config`. `FlavorPlatform` uses `Authorization: Token` and maps every selected dimension to the matching `app_id`, `user_id`, `agent_id`, or `run_id`. Mem0 OSS does not expose `app_id`, so `FlavorSelfHosted` encodes the complete four-dimensional Scope into one reserved native `user_id`; it uses `X-API-Key` when a key is supplied. This keeps Workspace App isolation exact without overwriting the caller's logical User, Agent, or Run dimensions. Update and delete retrieve the provider record and verify its complete encoded scope before performing the ID mutation. Self-hosted direct import accepts 1–1000 Facts with a non-empty Observation ID in one HTTP request, preserving each original text and attributes. Platform and Volc still accept one direct Fact and reject multiple candidates with `ErrUnsupported`.

Volcengine AgentKit/Viking MEM0 is constructed with one `volc.Config`. It accepts either an explicit Mem0 data-plane key or a credential resolver. The adapter explicitly selects Volc's v1 add/search routes, extracts one authoritative job ID from `results`, and makes `Wait` poll `/v1/job/{id}/`. If a successful job omits facts, it lists only the same scope and selects records using the per-operation reconciliation marker. Successful extraction may return zero facts, for example for a greeting; older memories are not returned as this job's results, and missing or malformed list responses still fail. The Volc v1 service requires `user_id`, so an App-, Agent-, or Run-only logical scope receives a reserved encoded transport user while retaining every original native field; returned records are decoded and checked against the unchanged logical scope. Generic Mem0 Platform continues to use v3 add/search, its top-level event ID, and `/v1/event/{id}/`. Endpoint hostnames are never used to infer the protocol. A Volc data-plane endpoint is mandatory.

An Eino `memory_observe` node assigns each Graph-authored direct Fact a stable observation identity derived from the current turn and Graph node. A following `memory_recall` node in the same Workspace therefore reads the Fact through the same complete `Scope.AppID`, including with `volc_mem0`. Volc search results must have a native Fact ID and a compatible encoded scope; provider routing metadata such as project and strategy bookkeeping is not returned as business attributes.

### Self-hosted Mem0 service

`cmd/mem0/gizclaw_mem0` is a standalone Python HTTP entry point using the official
`mem0ai 2.2.1` SDK. It supports OpenAI-compatible LLM/embedding providers with
PGVector or local Qdrant. `build/mem0/Dockerfile` provides `test` and non-root
`runtime` targets. Docker E2E, LoCoMo and Release share this implementation.
Extraction and embedding models belong to the service configuration;
RuntimeProfile stores only the endpoint and optional API key.

Select a YAML/JSON file with `--config` or `MEM0_CONFIG`. Its `memory` object uses
native Mem0 model/storage settings and rejects global business `custom_instructions`; `service` accepts `api_key`, `thinking` and
`embedding_protocol` and `max_concurrency`. Model QoS belongs to `memory.llm.config.service_tier`; the wrapper consumes this extension before constructing native Mem0 config and forwards it through the OpenAI SDK. `${VARIABLE}` substitutes process environment values and
fails startup for missing/empty values. File mode takes its model/database
configuration from that file instead of mixing in equivalent `MEM0_*` settings.
Without a file, environment configuration remains available.

- `cmd/mem0/config.example.yaml` selects Seed 2.1 Lite, Doubao Vision Embedding
  and PGVector. `service.thinking: disabled` forwards Ark's parameter through
  the OpenAI-compatible Chat API.
- The domestic example sets `memory.llm.config.service_tier: fast` for Ark's low-latency Chat tier.
  Environment mode uses `MEM0_LLM_SERVICE_TIER=fast`. Health reports the requested
  tier; extraction diagnostics record the actual response tier. The account must
  enable low-latency service. Quota/traffic protection can fall back to default;
  fast does not bypass model-account TPM limits.
- OpenAI also supports Fast mode: supported models accept `service_tier: fast` or `priority`. Account/model support follows the [official documentation](https://developers.openai.com/api/docs/guides/fast-mode#configuring-fast-mode). The OpenAI example uses `auto`.
- `cmd/mem0/config.openai.example.yaml` selects `gpt-6-luna`,
  `text-embedding-3-small` and PGVector. Luna maps `max_tokens` to
  `max_completion_tokens` and uses `reasoning_effort: none`; other reasoning
  modes omit incompatible sampling parameters. Request formatting is tested
  locally; account access and actual model quality still require live validation.
- `embedding_protocol: openai` uses standard `/embeddings`; `ark_multimodal`
  uses `/embeddings/multimodal`, produces one 1024/2048-dimensional vector per
  text and distinguishes corpus/query instructions. A model or dimension change
  requires a new collection and reingestion.

Run Python from the repository root after injecting the example's model key,
`MEM0_POSTGRES_DSN` and `MEM0_API_KEY`. PostgreSQL must support the `vector`
extension:

```sh
python3 -m venv .tmp/mem0-venv
.tmp/mem0-venv/bin/pip install -r cmd/mem0/requirements.txt
mkdir -p .tmp/mem0
export MEM0_HISTORY_DB_PATH="$PWD/.tmp/mem0/memory-history.db"
PYTHONPATH=cmd/mem0 .tmp/mem0-venv/bin/python -m gizclaw_mem0 \
  --config cmd/mem0/config.example.yaml --host 127.0.0.1 --port 8000
```

Build/run the container from the repository root with credentials already in the
process environment. With managed PostgreSQL, records and vector indexes live in
PG; `/data` holds the SDK's SQLite history and needs a persistent volume:

```sh
PLATFORM=linux/amd64 IMAGE=gizclaw-mem0 build/build-mem0.sh
# Domestic dependency source, same application Dockerfile with a CN base
PLATFORM=linux/amd64 IMAGE=gizclaw-mem0 build/build-mem0.sh cn
docker run --rm --name gizclaw-mem0 -p 127.0.0.1:8000:8000 \
  -e VOLC_ARK_API_KEY -e MEM0_POSTGRES_DSN -e MEM0_API_KEY \
  -v "$PWD/cmd/mem0/config.example.yaml:/app/config.yaml:ro" \
  -v gizclaw-mem0-history:/data gizclaw-mem0 --config /app/config.yaml --host 0.0.0.0
```

Python binds loopback by default. The container binds `0.0.0.0` internally, while
this example publishes only host loopback. When an API key is configured, every
route except `/health` requires `X-API-Key`. Local mode without a key belongs on
a trusted network. Shutdown closes model clients, the vector store and history
connections. Model/database settings cannot be changed through HTTP. PG observations can overlap across independent entities. Within this service process, writes and purges sharing any native entity are serialized, including compound writes and broader purges. Purges and ID updates/deletes also share a resolution guard acquired before reading the ID record, preventing a purge between entity resolution and mutation. These maintenance operations are serialized with each other while independent observations remain concurrent. LLM extraction is validated before SDK persistence, so invalid or incomplete extraction cannot commit memories before returning an extraction failure. Extraction diagnostics and embedding failures are isolated by
request thread. Local Qdrant remains serialized. `max_concurrency` is the write budget, defaulting to 160. Reads reserve a separate
`max(16, max_concurrency/4)` budget. Excess requests return 503; health bypasses
admission. Provider rate limits return structured HTTP 429; other model request
failures return 502 without forwarding provider credentials. The PG pool defaults to min 4 / max 32. Environment mode supports
`MEM0_MAX_CONCURRENCY`, `MEM0_POSTGRES_MIN_CONNECTIONS` and
`MEM0_POSTGRES_MAX_CONNECTIONS`. LoCoMo validates quality; the separate load
test validates throughput.

Keyed self-hosted `POST /memories` direct batches use paired `observation_id` and
`observation_digest` plus per-message `metadata`, with `infer=false`. The adapter
passes the canonical Observation digest, including text, attributes, order and
`ObservedAt`. The service reserves the immutable wire payload before writing;
changed payloads return HTTP 409 / `ErrConflict`. Legacy unkeyed direct HTTP calls
and `infer=true` extraction retain their behavior. Per-message metadata requires
a keyed direct request; provider-owned routing/history metadata cannot be overridden.

PostgreSQL stores reservations and completed Fact IDs in
`gizclaw_direct_observations`, keyed by collection, complete native routing and
observation ID. Native entity advisory locks coordinate independent service
processes. Embedded Qdrant remains owned by one process and stores reservations
in an `.observations.db` sidecar beside history. Each missing candidate uses the
official `Memory.add(infer=false)` with separate metadata. Exact vector listing
by routing/observation/index verifies persistence without semantic ranking.
Success requires every candidate to resolve uniquely. Lost replies, partial
writes and process restarts resume only missing candidates. Legacy single direct records reconcile their existing observation/digest and retain their original Fact ID. Explicitly deleted
completed facts are not recreated by retries; those retries conflict. Scope
purge removes matching reservations as well as facts. Batches are resumable,
not atomic: partial facts can be visible after an error until retry or purge.

`api/http/mem0.json` owns the HTTP contract, served at `/openapi.json`. Python
contract tests compare the actual request/response models, parameters, routes and
operation IDs. The root module pins `oapi-codegen` for the generated
`sdk/go/mem0` client:

```sh
go generate ./sdk/go/mem0
go test ./sdk/go/mem0 ./pkgs/store/memory/mem0
```

The self-hosted adapter uses its generated client and request DTOs. Platform/Volc
keep their own protocols. The SDK preserves extra Mem0 record metadata.

Multiple MemoryLayouts can share one service. The Go logical Store retains
`MemoryLayout.mem0.custom_instructions` for that Layout generation and sends it
as HTTP `prompt` to native `Memory.add(prompt=...)`. It does not mutate the
shared SDK instruction. Updating a Layout leaves existing generations unchanged.
Direct Facts with `infer=false` carry no extraction instruction. The service needs
no separate Layout registry. `scope: workspace|peer` keeps the existing Scope
contract; a Layout itself adds no partition, so the same endpoint/collection and
complete Scope share memory. OSS bindings reject `custom_categories`, enabled
`decay` and enabled `multilingual` flags.

See [repository publication](../tooling#repository-publication) for versioned
amd64/arm64 images.

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

The Server opens that physical backend once per binding and gives every Workspace generation an independently closable logical Store. Logical Store construction for different Workspaces proceeds concurrently when the published Flowcraft projection signature is unchanged; it is not part of the binding-registry map critical section. A policy change still has one serialized projection-rebuild owner. Other constructors continue only after the complete replacement is atomically published. A Resolve reservation retains the binding before it leaves the registry lock. Normal final-lease cleanup closes the physical backend after the last logical lease and in-flight Resolve have left; explicit Registry shutdown detaches the binding, rejects late constructor results, drains those constructors, and then closes the physical backend.

The valid Flowcraft connections are managed local `flowcraft_bbh`, `flowcraft_object_store` (`directory`), `flowcraft_postgresql` (`dsn`), and `flowcraft_redis8` (`url`, optional `tls_ca_file`). `flowcraft_bbh` needs no external service and stores each binding under `<server-root>/data/memory/<profile-id-hash>/<binding>`; optional `flowcraft.bbh` Layout policy controls BBH search overfetch, Bleve analysis, and HNSW flushing. `flowcraft_redis8` requires Redis 8.4 or newer with Redis Search and keeps canonical Facts, Evidence, the Async Semantic Queue, the Side-effect Outbox, and text/vector retrieval in one Redis namespace; it does not fall back to Redis 7 or Redis 8.0/8.2. Retrieval uses Redis-native BM25, HNSW KNN, structured metadata filtering, top-K limiting, and `FT.HYBRID` RRF fusion. `rediss://` reuses Storage TLS verification and `tls_ca_file` can add a trusted CA. Flowcraft 0.1.7 does not expose a Graph store injection port, so the Redis8 connection rejects `graph_enabled` instead of silently using a non-durable process-local Graph. Driver and connection type must match. Unknown fields, missing keys, and invalid endpoints are rejected when a RuntimeProfile is written or resolved.

Flowcraft 0.1.7 defines `(runtime_id, user_id)` as the canonical hard partition. `agent_id` is soft-isolation metadata and is intentionally excluded from `ScopeEnumerator`; enumerating that hard scope still reads facts written with every AgentID in the partition instead of fragmenting cross-agent recall.

There is no automatic migration from the removed `flowcraft_bbh` connection. Persisted legacy profiles fail closed with an operator-actionable replacement error, while mutation paths still allow the profile to be replaced with a supported connection. The old managed directory and its canonical data are retained unchanged; profile replacement and deletion do not remove them. `flowcraft_object_store` may continue using its local derived index internally, but BBH is not a public deployment connection or policy surface.

For Mem0 and Volc, the Project ID records the deployment/control-plane identity paired with the selected data-plane API key. Runtime fact requests authenticate with that key; they do not send a separate Project ID field.

Self-hosted Mem0 uses `driver: mem0` with `connection.type: mem0_self_hosted` to explicitly select the OSS HTTP protocol. Only `endpoint` is required. When the service enables authentication, supply the optional `api_key`, sent through `X-API-Key`; omit it for a local service without authentication. A self-hosted connection does not accept `project_id`, a database DSN, or model configuration. The Mem0 service owns its vector store, models, and persistence. The existing `connection.type: mem0` still selects the Platform protocol and requires `project_id` and `api_key`; endpoint hostnames never select the protocol.

```yaml
spec:
  resources:
    memories:
      assistant-memory:
        layout_id: assistant-memory
        driver: mem0
        connection:
          type: mem0_self_hosted
          endpoint: http://127.0.0.1:18000
```

Self-hosted extraction includes `Turn.Speaker` and UTC `ObservedAt` in message content because the OSS parser ignores OpenAI message names. Missing turn time falls back to the Observation time. Platform/Volc content and `infer=false` direct Facts retain their original text.

This binding uses the MemoryLayout's `mem0.scope` for Workspace or Peer ownership and retains the OSS adapter's complete Scope encoding, reads, writes, updates, deletes, and purge verification.

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

Deleting a Workspace irreversibly purges its long-term memory in the current memory binding. After quiescing runtime, the Workspace deletion handler resolves the binding from the retained Workspace row, the Workflow `memory` alias, and the owner's current RuntimeProfile, calls `Registry.PurgeWorkspace` for `Scope{AppID: <Workspace ID>}`, and finalizes only after `Registry.WorkspaceMemoryEmpty` confirms nothing remains; residual memory returns retryable `memory_residual`, and the retry purges again. The purge opens the physical backend shared with runtime Stores in maintenance mode: it loads no model and rebuilds no derived index, so a purge does not depend on the owner's model catalog, and a stale local derived index keeps its published manifest for the next runtime Store to rebuild. When the Workspace, Workflow, owner RuntimeProfile, memory alias, or MemoryLayout no longer exists, there is no resolvable current binding and the handler has nothing to purge; transient resolver or provider failures stay retryable; a scope the provider cannot express stops as terminal `memory_cleanup_unsupported` for operator action. GizClaw does not record binding history, so data a Workspace wrote under an earlier driver, connection, or alias is not reached by this purge.

## Ownership and errors

Provider adapters do not close injected dependencies. The composition root that constructs a workspace, index, HTTP client, or credential dependency owns it and closes resources in reverse construction order.

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior. If a provider cannot preserve a filter, attribute patch, or conditional-write semantic, it returns `ErrUnsupported` rather than discarding the condition. Errors must not expose API keys, access-key credentials, or credential-bearing response bodies.

Physical Memory Store construction is coordinated per complete binding key: callers for the same key share one backend, while unrelated bindings construct independently. Direct Mem0 Fact idempotency is coordinated by complete canonical Scope plus observation ID, so a slow provider request does not stop unrelated scopes or observations; same-key retries still reconcile or return `ErrConflict`.
