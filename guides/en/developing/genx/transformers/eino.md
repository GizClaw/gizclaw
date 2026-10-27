# Eino Transformer

[Graph definitions and validation](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/transformers/eino)

`pkgs/genx/transformers/eino` builds a typed Eino Graph and exposes it as a concurrently reusable `genx.Transformer`. The package owns Graph construction, state, streaming, History, Memory, and turn lifecycle. It depends on GenX, Eino core, Starlark, and generic Stores; it does not import GizClaw Workspace, Workflow, Resource, AgentHost, or product Toolkit types.

## Construction and ownership

```go
transformer, err := eino.New(ctx, eino.Config{
    Agent: eino.AgentConfig{
        ID:        "assistant",
        Name:      "Assistant",
        ContextID: "workspace/assistant",
    },
    Graph:      graph,
    Components: components,
    Lambdas:    lambdas,
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
    Limits:     eino.Limits{MaxOutputBytes: 4 << 20},
})
```

`New` validates and copies the declarative configuration, resolves every referenced component and named Lambda, constructs native Eino nodes and routing, compiles the root Graph and every nested Graph once, and returns an immutable Transformer. It does not connect to a provider, start a permanent worker, or mutate global registration.

`ComponentResolver`, `LambdaResolver`, resolved components, and Stores remain caller-owned and must be safe for concurrent use. `Config` never accepts a preconstructed Agent, Runnable, mutable `compose.Graph`, Graph factory, raw graph callback, credential, provider endpoint, or product Resource.

An empty `Agent.ContextID` creates one opaque identity during `New`. That identity is stable for the Transformer lifetime. Each turn still receives fresh invocation, run, and output Stream identities.

Each output route receives an immutable `ResponseEpoch` when its turn starts,
bound to that turn's input StreamID. A lazily created `first_output` route uses
that preallocated epoch too. Content and terminal events keep the same epoch,
so a consumer observing the first chunk after replacement input can still
identify the original owner. Initiative without an input route remains unowned.
This metadata stays inside the process.

## Graph contract

`GraphDefinition` combines State, nodes, routing, and outputs.

Binding namespaces and types are owned by the Graph definitions linked above.

Node input maps use component port names as keys. Node output maps use node output-port names as keys and declared State fields as values. Unknown bindings, fields, ports, or incompatible types fail in `New`.

State supports `string`, `boolean`, `integer`, `number`, `object`, `list`, `messages`, `documents`, and `blob`. `replace` applies to every type. `append` applies to string, list, messages, and documents. `object_merge` applies only to objects and replaces an existing key with the later sequential value.

Parallel writers to one State field are rejected unless they are ordered by Graph reachability or are direct, mutually exclusive destinations of one `first_match` branch. Use distinct fields followed by an explicit merge node for native fan-out.

## Routing and scheduling

Edges map to Eino `AddEdge`. `first_match` branches select the first matching route or `Default`; `all_match` branches select every matching route and use `Default` only when no route matches. Predicates support recursive `all`, `any`, and `not`, plus existence, equality, containment, and numeric comparisons.

`NodeTriggerAnyPredecessor` uses Eino's any-predecessor scheduler. `NodeTriggerAllPredecessor` creates a join barrier and accepts only acyclic Graphs. A cyclic any-predecessor Graph requires a positive `MaxRunSteps`.

`GraphCompileConfig.FanIn` maps only to Eino's `FanInMergeConfig.StreamMergeWithSourceEOF`. Each configured node must exist and have at least two predecessors. State value merging remains the responsibility of declared State merge policies and explicit nodes.

Native parallelism executes sibling Graph paths and joins them through Eino scheduling. `Race` is different: it runs isolated nested Graphs, selects one winner, cancels losers, and merges only the winning Graph outputs. Use native fan-out when all selected work is required; use `Race` when only one result may survive.

## Supported nodes and ports

Each Prompt message uses exactly one declaration form. A role message declares a non-empty `role` and `template`. A placeholder message declares a non-empty `placeholder` and may set `optional`; it cannot also declare `role` or `template`. The placeholder name must identify a Prompt input bound to `messages`.

Prompt, ChatModel, and Retriever components are added through Eino's native `AddChatTemplateNode`, `AddChatModelNode`, and `AddRetrieverNode` paths inside typed nested Graphs. Package-owned Transform, Script, Race, Batch, and State adapters use Eino Lambdas when no serializable native component contract exists.

`ChatModel.Temperature` is optional. An explicit `0` reaches the resolved component as an Eino model option, and GizClaw's GenX bridge preserves that zero. Omitting temperature leaves the component default in effect.

ChatModel uses the resolved Eino streaming interface. Text chunks are published incrementally when the model node owns a declared text output. When a `ToolInvoker` is configured, `ResolveTools` supplies function names, descriptions, and schemas through Eino model options. Correlated ToolCalls execute in model order through `InvokeTool(name, arguments)`, native tool messages are appended, and the same model node continues. Internal calls and results are not published. A requested text port fails when the completed model turn contains no text.

A host can also configure `Config.VerifyToolResponse`, receiving the actual
`genx.ToolConversation` snapshot and final reply draft. Empty feedback accepts it;
nonempty feedback permits at most two native-model corrections. The Transformer
does not generate Tool names or arguments. This option requires `ToolInvoker` and
buffers reply text: intermediate Tool rounds and rejected drafts are withheld,
and an accepted final reply is published once. Callback failure or exhausted
corrections ends the turn with an error without leaking unverified text. Only
messages after `ContinuationStart` contain current execution evidence; historical
success is not proof of a new operation. Call correlation, budget and completed
results survive correction, while each invocation owns a separate snapshot.
Product authorization and semantics remain the host's responsibility.

Tool-result snapshots associate actual arguments using the internal call ID and matching function
name, then omit the ID. Multiple proposals with the same name or reordered results cannot select
arguments by name; an unmatched result receives no invented arguments.

### Structured control

A ChatModel can set `StructuredOutput` when its resolved component implements
`StructuredChatModel`. GizClaw exposes the same option as
`chat_model.structured_output` in the [Workflow source contract](https://github.com/GizClaw/gizclaw/blob/main/api/http/shared/workflows/eino.json).
It uses the resolved GenX Generator's native `Invoke` capability, carrying the
same typed messages, original audio, History, model options and cancellation.
Provider JSON output or native function calling supplies the object; the node
does not advertise or execute runtime Tools. A provider error, truncated or
malformed JSON, schema mismatch or missing component capability fails without
repairing text or falling back to ordinary streamed generation.

```yaml
structured_output:
  name: turn_intent
  schema:
    type: object
    additionalProperties: false
    required: [mode]
    properties:
      mode:
        type: string
        enum: [story, research]
```

The complete object is checked against the snapshotted schema and
`MaxOutputBytes` before it can update State or publish. A published structured
node emits its validated JSON once. For internal control, omit that node from
Graph outputs and publish only the subsequent reply node; control results stay
out of client text, speech and conversational History. Existing State fields
can be reused by sequential nodes. `VerifyToolResponse` is incompatible with
structured nodes and is rejected during construction instead of being bypassed.
Omitting this option preserves normal incremental replies and Tool continuations.

### Audio turns

The host enables complete audio turns with `Config.AudioInput`. Original audio enters the Graph through typed `input.messages`. When the host supplies an `InputText(ctx)` callback, declared `input.text` bindings resolve the same input's final transcription using the node context outside the State lock; independent response paths can stream before that resolution. Without the callback, `input.text` is empty. ChatModel generates replies without transcribing input. Otherwise the Transformer consumes text turns from upstream ASR.

An audio MIME EOS starts the turn; its following successful control EOS is consumed. A new audio route interrupts the previous turn, an `interrupted` EOS discards the turn, and other errors propagate through the Stream. Each Blob remains an audio part of the current user message. Recall using `input.text` skips an audio turn; recall with an independent text query can still run.

The surrounding [AudioDock](../agentkit#audio-dock) and host History own ASR, user transcripts and original audio recording. Eino publishes no user transcription and makes no additional transcription request. GizClaw's `spec.eino.audio_input` selects `auto`, `asr` or `model`. Explicit `model` enables supported PTT Graphs with text-dependent functions through the callback; omitted or `auto` retains automatic selection, and continuous realtime uses the ASR text path.

### Match

`MatchNode` compiles the shared `pkgs/genx/match` rules during `New`, resolves its model alias once through `ComponentResolver.ResolveChatModel`, and sends exactly one system message and one user string to that model. It does not advertise or execute tools.

```go
eino.NodeDefinition{
    ID: "route",
    Inputs: map[string]eino.Binding{
        "text": {From: "input.text"},
    },
    Outputs: map[string]string{
        "matches": "route_matches",
    },
    Match: &eino.MatchNode{
        Model: "router",
        Rules: []*match.Rule{{
            Name: "play_music",
            Vars: map[string]match.Var{
                "title": {Label: "Song title", Type: "string"},
            },
            Patterns: []match.Pattern{{Input: "Play [title]"}},
        }},
    },
}
```

`text` accepts `input.text` or a declared string State field. `matches` must target a declared `StateList` field. Empty text is valid. The node consumes all model chunks through the shared Match parser and writes the JSON-compatible ordered result only after the stream completes successfully; a model, stream, parsing, or cancellation error publishes no partial State value. The same compiled node may run concurrently and inside Subgraph, Race, or Batch.

The built-in Transform operations are:

- `select`: one `value` input and output with the same type;
- `concat_text`: string inputs listed exactly once in `Order`, optional `Separator`, and one `text` output;
- `decode_json`: one `text` input, one `object` output, positive byte limits, UTF-8 object JSON, and duplicate-key rejection; and
- `build_messages`: ordered system, user, or assistant items using either literal text or declared string input, and one `messages` output.

In `f_string` system templates, escape literal braces or use brace-free wording. Unescaped empty braces also participate in interpolation. User history belongs in a separate message placeholder and must not be interpolated into static system rules.

## Starlark Script

Script source is compiled once and its initialization is validated in `New`. Every run initializes fresh module globals under the configured step, timeout, and cancellation limits before calling the entrypoint, so mutable globals cannot cross turn boundaries. The entrypoint defaults to `run` and receives one frozen dictionary:

```go
eino.ScriptNode{
    Language: eino.ScriptStarlark,
    Source: `
def run(input):
    return {
        "answer": input["question"] + "!",
        "labels": ["scripted", "bounded"],
    }
`,
    Limits: eino.ScriptLimits{
        MaxExecutionSteps: 10_000,
        Timeout:           250 * time.Millisecond,
        MaxInputBytes:     64 << 10,
        MaxOutputBytes:    64 << 10,
    },
}
```

The return value must contain exactly the declared output keys. Supported values are null, boolean, integer, finite number, text, list, object, messages, documents, and binary. Binary values use base64 text inside Starlark. Messages use `{"role": "...", "content": "..."}` objects; text segments from multimodal messages are also available in `parts`. Documents use `id`, `content`, and optional `metadata`.

Every Script limit must be positive. Step exhaustion, timeout, cancellation, malformed source, runtime failure, byte-limit failure, unsupported conversion, missing output, or undeclared output terminates the Graph run. The sandbox has no file, network, environment, process, random, Store, Tool, Graph, or native Go access.

Starlark provides `json.encode` / `json.decode`, bounded RE2 `regex_find` / `regex_replace`, and `now_millis()`. `regex_find` caps global matches and captures at 4096, enforces the configured output-byte budget before constructing the result, and checks cancellation. `regex_replace` bounds match-index storage and capture counts by the output budget, then checks cancellation and remaining bytes before appending each replacement segment. Numeric/named captures and `$$` retain RE2 expansion semantics; overflow returns an error. The clock returns Unix milliseconds. Scenarios must persist their date and random decisions explicitly in State to retain them after reload.

## Named Lambda

A named Lambda keeps Go behavior outside serializable configuration:

The resolver runs during `New`. The descriptor must match every configured port and State type. The Lambda ABI is `map[string]any` input to `map[string]any` output; use `compose.InvokableLambda` or another Eino Lambda with that shape. Returned values must contain every declared output port. Lambdas are caller-owned and must be safe for concurrent invocation.

## Race, Batch, and Subgraph examples

Race branches declare complete nested Graphs with identical output schemas:

```go
eino.RaceNode{
    Branches: []eino.RaceBranch{
        {ID: "fast", Graph: fastGraph},
        {ID: "accurate", Graph: accurateGraph},
    },
    Winner: eino.RaceWinnerDefinition{
        Mode: eino.RaceFirstSuccess,
    },
    MaxConcurrency: 2,
}
```

Winner modes are `first_output`, `first_success`, and `predicate`. `first_output` selects a winner and cancels losing branch contexts as soon as the first declared output is emitted, while the winner finishes its owned output. Predicate mode evaluates the configured predicate against each completed child State. Child State and output buffers are isolated; the winning named Graph outputs are copied to the parent.

Batch applies one nested Graph to an ordered list:

```go
eino.BatchNode{
    Items:          eino.Binding{From: "documents"},
    Graph:          itemGraph,
    MaxConcurrency: 4,
}
```

Each item initializes the child State field named `item`. Execution is bounded and fail-fast. The result contains one `items` list in original input order; an error returns no partial list.

Subgraph executes one nested Graph once. Inputs named `text`, `messages`, and `parts` initialize the corresponding child input namespace; other input names initialize same-named child State fields. All nested Graphs are validated and compiled during `New`.

## Outputs and Stream lifecycle

`Outputs` is the only publication allow-list. Each output names one node-produced string or blob State field, route name, and MIME type. Names and node-field sources are unique, and exactly one output is primary.

Each completed input text turn creates fresh output routes and StreamIDs. Every route receives BOS, data, and its own EOS. Model text may arrive incrementally while the Graph is still running. Non-primary routes finish first in stable name order; successful primary EOS is the last boundary.

The output buffer grows independently of downstream pulls up to `Limits.MaxOutputBytes`. Crossing the limit fails all routes. A new text BOS interrupts the previous turn, cancels its Graph and children, discards unpulled suffixes, and preserves only the assistant prefix already observed by downstream.

An upstream text EOS with a non-empty StreamID and the exact error `interrupted` is a turn-scoped replacement terminal only when it matches the active incomplete input route or a route that a replacement BOS explicitly superseded. Eino discards buffered text and parts for the active match and ignores a known superseded route's terminal as stale. Unknown or mismatched StreamIDs retain their validation errors. The session tracks at most 64 superseded routes awaiting terminals; exceeding that bound fails the session instead of growing replacement state without limit. The Transformer session stays open for valid replacements, and the replacement BOS remains the only owner of active-Graph interruption. Every other non-empty input terminal error remains fatal to the session.

Apart from the audio turns above, non-text routes bypass the Transformer unchanged. A text turn containing blobs is accepted only when the Graph explicitly binds `input.parts`; otherwise it fails as unsupported multimodal input. Component-specific interpretation of those copied parts remains outside the package.

`Compile.PrimaryOutputMode` defaults to `fixed`: every successful path must pass through the declared primary node. In `first_output` mode, the first output actually published in a turn becomes primary. Only executed outputs publish BOS/EOS, retain their configured Name, and carry the `assistant` label. Empty text is a valid publication. A path with no publication fails even if persisted State contains older values. Other executed outputs finish before the primary EOS. History and Memory retain all delivered output text and wait for its delivery boundary.

## State, History, and Memory

### Continuous narration within one turn

`Config.ContinueFrom` names a boolean root State field with `replace` merge. After each successful Graph execution, true requests another generation within the same user turn; false ends narration. The field resets to false before each execution, so the Graph must explicitly request continuation. Each root Graph generation has independent `MaxRunSteps` and `MaxToolCalls` budgets and a fresh call-ID set; narration has no fixed iteration count. Tools such as a search for every section do not accumulate against one default 32-call budget for the entire narration.

This mode requires one `text/plain` string output with `replace` merge and text input to the Graph; audio may be transcribed beforehand. Every execution must publish fresh, non-whitespace narration, including the terminal execution that sets `ContinueFrom` to false. Skipped or blank output ends with error EOS and does not checkpoint that generation. State is retained, and `input.messages` contains the original input followed by the last eight generations without synthetic user messages. Each generation has its own process-local `StreamCtrl.MessageID`; `MessageEnd` ends that message's MIME channel while the playback StreamID and BOS/EOS stay continuous.

With a caller-owned History reader, the Host records one authoritative assistant History entry per delivered generation, pairing its text and audio for replay; it records the original user message only once. Eino reads those entries without appending another durable copy. Without a reader, Eino keeps bounded local History. User interruption cancels active generation, discards undelivered suffixes, and records the current generation's delivered prefix separately. Completed entries are neither merged nor rewritten. Resuming or redirecting creates a new user turn, still saving one entry per generation.

The Transformer starts another generation only after text delivery and message-boundary acknowledgement. For Voice output, the product factory enables [Audio Dock backpressure](../agentkit#continuous-narration-backpressure) to wait for corresponding TTS audio delivery and prevent sustained accumulation; `speaker_voices` is unsupported in this mode. Delivered text is released after each generation, continuation context retains eight sections, and `Limits.MaxOutputBytes` bounds queued Transformer output. Memory observation uses a separate identity per message.

With persistent State configured, each generation commits only the fields selected by `StatePersistenceConfig.Fields` after complete delivery, `MessageEnd` acknowledgement and successful internal Memory processing and local History bookkeeping when no reader is configured. With an external History reader, the Host attempts the sole durable History write before forwarding acknowledgement; its best-effort append failure is logged and does not gate State CAS. The returned CAS version is used for the next generation; final turn cleanup does not repeat the CAS. Cached queues and playback cursors can therefore checkpoint during continuous narration. Interrupted, failed or unacknowledged generations do not commit those fields; resumption loads the last successful checkpoint. A CAS conflict terminates with an error EOS, preserving the concurrent writer's state and earlier completed checkpoints. Successful Host History recording precedes State CAS, but the two stores are not one atomic transaction; History already saved before a failed CAS is not rolled back.

Product Workflows select the State field with `spec.eino.conversation.continue_from`. Graphs should generate short, naturally connected sections ending in complete sentences without additional user “continue” turns.

Product Workflows select persisted fields with `state_persistence.fields`. Server configuration `services.agent_host.persistence.state_store` references a SQL Store: `graph_states` stores snapshots and `graph_state_scopes` retains deletion fences. Missing selected fields receive their declared typed zero value on first load. Reload restores only selected fields. Nested object/list integers retain signed 64-bit precision, and integral floating-point values retain their numeric type through optional snapshot type hints. New snapshots use format version 1; unversioned snapshots retain their previous JSON float64 decoding until a normal successful CAS write. Conversation History reuses the Workspace `memory.HistoryStore`; Eino only reads it.

Persistent State is optional:

```go
State: &eino.StatePersistenceConfig{
    Store:  stateStore,
    Scope:  "workspace/assistant",
    Fields: []string{"summary", "turn_count"},
}
```

The Store loads one versioned snapshot before the Graph. Only configured fields are validated and copied into fresh local State. Ordinary turns commit final configured fields with compare-and-swap before successful primary EOS; continuous generations commit at the delivery boundary described above. Failure, cancellation, interruption, invalid State or conflict does not commit the current failed generation or overwrite completed checkpoints.

History accepts an optional `memory.HistoryReader`, selects its conversation through `Scope`, and reads recent user/assistant text with a bounded limit. External Reader mode never calls Append or persists a second history. The host records completed raw text input, ASR transcripts, and delivered replies, and provides the write-completion barrier before the next read. `ExcludeTurnID` excludes the current input, which is added separately to the model request. Without a Reader, History stays bounded and Agent-local.

Memory uses an optional provider-neutral `memory.Store`. Each Recall declaration runs before the Graph, renders ordered facts as `- text` lines, writes its string State field, and contributes to `memory.recalled`. A Recall whose query text is blank, as in an agent-initiative turn, skips the Store and writes an empty result instead of failing the run. Observe runs after delivery observation. It submits pull-visible turns and explicitly declared State fact bindings only.

When `WaitForCompletion` is true, the Store must implement `memory.OperationWaiter`, and primary EOS waits for terminal operation success. When false, Observe acceptance still precedes EOS; a Store implementing `memory.AsyncOperationProcessor` may process a pending operation asynchronously.

## Validation and errors

`New` rejects invalid names, node unions, State types, merges, ports, bindings, component schemas, output MIME, duplicate output ownership, unreachable nodes, nodes that cannot reach `end`, unknown routing targets, impossible fan-in, concurrent writers, invalid cycles, recursive nesting deeper than 16 levels, and partial Store configuration. Errors include the Graph path and offending node or field.

Runtime provider, Store, Script, component, cancellation, byte-limit, and optimistic-concurrency failures terminate every active route with an error EOS. A failed ordinary turn or failed continuous generation does not commit persistent State; earlier completely delivered and committed continuous generations remain saved.

The Eino Transformer depends only on the GenX `ToolInvoker` interface and does not receive RuntimeProfile, Toolkit policy, resource, or executor-registry details. All ChatModel nodes, model tool continuations and nested Graphs within one root Graph generation share a call-ID set and `MaxToolCalls` budget. Provider call IDs remain inside Eino and are associated with the raw JSON result returned by `InvokeTool`. Zero uses 32 and negative values are rejected. The next root Graph generation requested by `ContinueFrom` receives a fresh bounded budget and call-ID set, permitting IDs from the preceding section to be reused; duplicate IDs and excess calls within one generation still fail before dispatch to `ToolInvoker`. An existing tool scope inherited through the calling context stays shared, so child Transformers and internal continuation cannot bypass an ancestor's limit. Independent invocations may execute the shared invoker concurrently; resolution, invocation, invalid-result JSON, cancellation, duplicate-ID, and exhaustion failures remain local to one invocation.

For agent initiative, ChatModel omits content-free user messages rendered by Prompt while preserving system instructions, history, and multimodal input.

## Realtime speech endpointing

Eino realtime speech input explicitly sets Volc ASR `end_window_size=200` and
`force_to_speech_time=1000`. The former shortens endpointing after speech while
the latter preserves the minimum speech duration. Push-to-Talk keeps the ASR
Builder defaults. The model still starts only after definite ASR text EOS;
interim transcripts do not trigger a dialogue turn.

With final-reply verification enabled, malformed model Tool JSON can request regeneration before dispatch. Syntax and reply corrections share the same two-correction budget. Invalid proposals enter neither invocation nor wire history; parameters are not patched and no result is fabricated. Persistent errors or disabled verification still fail closed.
