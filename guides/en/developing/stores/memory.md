# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) is the provider-neutral long-term memory boundary used by Agent runtimes. Mem0 Cloud, self-hosted Mem0, and Volc adapters live in the `mem0` and `volc` subpackages.

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

| Common field | Mem0 / Volc Memory |
| --- | --- |
| `AppID` | `app_id` |
| `UserID` | `user_id` |
| `AgentID` | `agent_id` |
| `RunID` | `run_id` |

Mem0/Volc preserve these independent dimensions and their combinations, send every selected dimension, and use AND filters for recall and mutation checks. See the self-hosted complete-Scope encoding below.

`Text` and `Turns` are raw extraction material; `Facts` are candidates already structured by the caller. A provider must preserve candidate text and supported attributes or return `ErrUnsupported`; it must not silently send candidates through model extraction again. Mem0 and Volc use `infer=false` direct import and preserve candidate text and supported attributes.

For an Observation containing direct Facts, a non-empty `Observation.ID` is an idempotency key within the complete `Scope`. Concurrent calls or retries with the same ID and canonical direct-Fact payload return the original logical Fact or durable operation. Changing Fact text, attributes, or `ObservedAt` returns `ErrConflict`. Adapters retain a payload digest in native records and reconcile before submission, so retrying after a provider accepted a request but lost its response does not create a second logical Fact. Returned `Fact.Sources` retain the `ObservationID`; provider-owned metadata is not exposed as business attributes. Provider-native deduplication for model extraction is outside this direct-Fact guarantee.

`UpdateRequest`, `DeleteRequest`, and `OperationRequest` resubmit the caller's `Scope` together with an opaque fact, revision, or operation locator returned by the Store. A locator is not an authorization source: before a mutation or asynchronous completion is accepted, the adapter verifies that the requested Scope matches the locator and provider record. A raw provider ID cannot bypass an App boundary.

Asynchronous `Observe` returns an operation. Stores implementing `OperationWaiter` use the caller context and validate the locator against the complete Scope when restoring an operation. Constructors do not start background workers.

`memory.BindApp(store, appID)` returns a borrowed Store view. It only fills or verifies `Scope.AppID`; it never generates, clears, concatenates, hashes, or rewrites caller-supplied `UserID`, `AgentID`, or `RunID`. A conflicting AppID returns `ErrInvalidInput`. The view does not own or close the underlying Store, and it exposes `OperationWaiter`, `AsyncOperationProcessor`, and `StatisticsProvider` only when the underlying Store provides the same capability.

`ScopePurger` is the optional capability that irreversibly removes one scope. The purge set is exactly the Facts that `Recall` with the same `Scope` can return, plus provider-owned records that could later materialize into that set (pending extraction jobs, derived indexes, and provenance markers). `PurgeScope` is idempotent; when native bulk deletion cannot express that set exactly it returns `ErrUnsupported` instead of deleting more or less. `ScopeEmpty` reports whether any Fact of the purge set remains. Callers use `memory.PurgeScope` and `memory.ScopeEmpty`, which unwrap `BindApp` views and apply their AppID binding first, and return `ErrUnsupported` when the underlying Store lacks the capability.

| Provider | Purge | Verification |
| --- | --- | --- |
| Mem0 Platform | `DELETE /v1/memories/` with the selected dimensions ANDed; a dimension whose value is `*` is a provider wildcard and returns `ErrInvalidInput` | `POST /v3/memories/` lists the same filter |
| Mem0 self-hosted | `DELETE /memories?user_id=<complete scope encoding>` | `GET /memories` with the same `user_id` |
| Volc | `DELETE /v1/memories/?user_id=<reserved scope user>`. Volc bulk delete accepts only `user_id`, `agent_id`, and `run_id` and cannot narrow a caller-selected `UserID` to one App, so such a scope returns `ErrUnsupported` | `GET /v1/memories/` with the dimensions used on write |

Mem0 Platform deletes asynchronously, and a Volc `async_mode` add job can materialize after a purge. Callers that need a durable guarantee purge and verify repeatedly until `ScopeEmpty` reports true.

## Provider construction

Provider packages accept in-memory runtime dependencies only. They do not decode YAML, expand environment variables, open configuration files, or choose product identity.

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

The service initializes the PGVector collection through the native SDK during startup, before accepting requests. This prevents concurrent cold search/add requests from racing table creation. A collection initialization failure aborts startup and closes constructed clients and connection pools.

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
`MemoryLayout.mem0_self_hosted.custom_instructions` for that Layout generation and sends it
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

Portable policy, deployment connection, and Graph consumption belong to three resource surfaces:

- Admin `MemoryLayout` declares Mem0 Cloud, self-hosted Mem0, and Volc policy without endpoints, keys, database or model connections. Each policy independently chooses `scope: workspace|peer`, defaulting to Workspace scope.
- RuntimeProfile `resources.memories.<alias>` selects a Layout, driver, and one typed connection. This Admin resource owns the connection; it does not reference a Credential or project it through Peer APIs.
- Workflow `memory` references a RuntimeProfile alias. Eino `memory_recall` / `memory_observe` nodes own query, filters, raw extraction material, and direct Facts.

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata:
  id: pet-memory
spec:
  mem0:
    scope: peer
    custom_instructions: Extract durable pet and owner facts.
  mem0_self_hosted:
    scope: peer
    custom_instructions: Extract durable pet and owner facts.
  volc_mem0:
    scope: peer
    strategies:
    - name: owner-profile
      type: user_preference
      custom_instructions: Extract durable pet and owner facts.
```

The `mem0` and `volc_mem0` blocks are required. The independent optional `mem0_self_hosted` block must be explicitly present for its connection type; `{}` is valid. It accepts only `scope` and `custom_instructions` and does not inherit Cloud categories, multilingual, or decay. Construction, reload, access, statistics, and cleanup select the same policy by connection type.

Supported connections are `mem0` (Cloud endpoint, API key, Project ID), `mem0_self_hosted` (endpoint, optional API key), and `volc_mem0` (endpoint, API key, Memory Project ID). Driver `mem0` accepts the first two; driver `volc_mem0` accepts its matching connection. Unknown fields, missing parameters, and driver/connection mismatches are rejected on write or resolution.

Self-hosted Mem0 explicitly selects the OSS protocol and sends enabled authentication via `X-API-Key`. Its service owns databases, extraction models, Embedding, and persistence. Cloud/Volc Project IDs identify the control plane paired with the key; Fact requests route through that key without a separate Project ID parameter. The hostname does not select the protocol.

```yaml
spec:
  resources:
    memories:
      pet-memory:
        layout_id: pet-memory
        driver: mem0
        connection:
          type: mem0_self_hosted
          endpoint: http://127.0.0.1:18000
```

Self-hosted extraction puts `Turn.Speaker` and UTC `ObservedAt` in message content, using Observation time when the turn has none. Cloud/Volc messages and direct Facts retain their original text.

This Eino Graph recalls, generates a reply, then includes the current assistant in raw conversation material before requesting extraction:

```yaml
spec:
  driver: eino
  memory: pet-memory
  eino:
    graph:
      name: companion
      compile:
        node_trigger_mode: any_predecessor
      state:
        fields:
        - name: memory_context
          type: string
          merge: replace
        - name: messages
          type: messages
          merge: replace
        - name: answer
          type: string
          merge: replace
        - name: turns
          type: messages
          merge: replace
      nodes:
      - id: recall
        type: memory_recall
        query_from: input.text
        output: memory_context
        top_k: 5
      - id: prompt
        type: prompt
        format: f_string
        inputs:
          memory:
            from: memory_context
          text:
            from: input.text
        outputs:
          messages: messages
        messages:
        - role: system
          template: '{memory}'
        - role: user
          template: '{text}'
      - id: answer
        type: chat_model
        model: chat
        inputs:
          messages:
            from: messages
        outputs:
          text: answer
      - id: conversation
        type: script
        language: starlark
        source: "def run(input):\n    return {\"turns\": list(input[\"messages\"]) + [{\"role\": \"assistant\"\
          , \"content\": input[\"answer\"]}]}\n"
        inputs:
          messages:
            from: input.messages
          answer:
            from: answer
        outputs:
          turns: turns
        limits:
          max_execution_steps: 1000
          timeout: 100ms
          max_input_bytes: 262144
          max_output_bytes: 262144
      - id: observe
        type: memory_observe
        turns_from: turns
        wait_for_completion: false
      edges:
      - from: start
        to: recall
      - from: recall
        to: prompt
      - from: prompt
        to: answer
      - from: answer
        to: conversation
      - from: conversation
        to: observe
      - from: observe
        to: end
      branches: []
      outputs:
      - node: answer
        field: answer
        name: assistant
        mime_type: text/plain
        primary: true
```

Registry construction coordinates by the complete binding key. Independent keys perform network work independently; short registry critical sections retain, publish, or retire entries. Each Workspace generation owns a separately releasable lease. Shutdown rejects late constructors and drains in-flight work before closing shared backends.

Workspace scope maps the Workspace ID to `Scope.AppID`; Peer scope derives a reserved `peer:<hash>` AppID from the owner's public key. User, Agent, and Run dimensions remain independent. Cloud/Volc preserve representable provider dimensions; self-hosted Mem0 encodes the complete Scope into transport `user_id`. Sharing Peer memory across Workspaces also requires the same provider project or self-hosted data space. Equal AppIDs do not merge different connections.

Workspace deletion purges only Workspace scope in the current binding; Peer deletion purges Peer scope. After quiescing runtime, the handler resolves the retained Workspace, Workflow alias, and owner's current Profile, then repeats purge and empty verification before finalizing. Residual data yields retryable `memory_residual`; an unrepresentable scope yields terminal `memory_cleanup_unsupported`. No resolvable current binding requires no cleanup, while transient provider/resolution failures remain retryable. Cleanup does not load extraction Models. GizClaw has no binding history and does not access previous sources; changing driver, binding, or scope neither migrates nor deletes old data.

## Ownership and errors

Provider adapters do not close injected dependencies. The composition root that constructs a workspace, index, HTTP client, or credential dependency owns it and closes resources in reverse construction order.

The stable sentinel errors are `ErrInvalidInput`, `ErrNotFound`, `ErrUnsupported`, `ErrConflict`, and `ErrUnavailable`. Providers preserve `errors.Is` behavior. If a provider cannot preserve a filter, attribute patch, or conditional-write semantic, it returns `ErrUnsupported` rather than discarding the condition. Errors must not expose API keys, access-key credentials, or credential-bearing response bodies.

Physical Memory Store construction is coordinated per complete binding key: callers for the same key share one backend, while unrelated bindings construct independently. Direct Mem0 Fact idempotency is coordinated by complete canonical Scope plus observation ID, so a slow provider request does not stop unrelated scopes or observations; same-key retries still reconcile or return `ErrConflict`.
