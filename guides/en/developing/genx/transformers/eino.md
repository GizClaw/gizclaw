# Eino Transformer

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

## Graph contract

`GraphDefinition` declares State fields, typed nodes, edges, branches, compile behavior, and explicit outputs:

```go
type GraphDefinition struct {
    Name     string
    Compile  GraphCompileConfig
    State    StateDefinition
    Nodes    []NodeDefinition
    Edges    []EdgeDefinition
    Branches []BranchDefinition
    Outputs  []OutputDefinition
}
```

Bindings use this closed namespace:

| Binding | Type | Value |
| --- | --- | --- |
| `input.text` | `string` | Completed user text turn. |
| `input.messages` | `messages` | Ordered History followed by the current user message; agent initiative uses History alone without an empty user message. |
| `input.parts` | `list` | Defensively copied non-text input parts. |
| `history.messages` | `messages` | Ordered prior History only. |
| `memory.recalled` | `string` | Combined rendered recall results. |
| `input.safety_fence` | `string` | Host safety fence text from `Config.SafetyFence`, empty when none is selected; batch, race, and subgraph runs inherit it. The transformer never places it itself, so a Graph that does not bind it is unaffected. |
| Bare State field name | Declared type | Current invocation-local State value. |

Node input maps use component port names as keys. Node output maps use node output-port names as keys and declared State fields as values. Unknown bindings, fields, ports, or incompatible types fail in `New`.

State supports `string`, `boolean`, `integer`, `number`, `object`, `list`, `messages`, `documents`, and `blob`. `replace` applies to every type. `append` applies to string, list, messages, and documents. `object_merge` applies only to objects and replaces an existing key with the later sequential value.

Parallel writers to one State field are rejected unless they are ordered by Graph reachability or are direct, mutually exclusive destinations of one `first_match` branch. Use distinct fields followed by an explicit merge node for native fan-out.

## Routing and scheduling

Edges map to Eino `AddEdge`. `first_match` branches select the first matching route or `Default`; `all_match` branches select every matching route and use `Default` only when no route matches. Predicates support recursive `all`, `any`, and `not`, plus existence, equality, containment, and numeric comparisons.

`NodeTriggerAnyPredecessor` uses Eino's any-predecessor scheduler. `NodeTriggerAllPredecessor` creates a join barrier and accepts only acyclic Graphs. A cyclic any-predecessor Graph requires a positive `MaxRunSteps`.

`GraphCompileConfig.FanIn` maps only to Eino's `FanInMergeConfig.StreamMergeWithSourceEOF`. Each configured node must exist and have at least two predecessors. State value merging remains the responsibility of declared State merge policies and explicit nodes.

Native parallelism executes sibling Graph paths and joins them through Eino scheduling. `Race` is different: it runs isolated nested Graphs, selects one winner, cancels losers, and merges only the winning Graph outputs. Use native fan-out when all selected work is required; use `Race` when only one result may survive.

## Supported nodes and ports

| Node | Inputs | Outputs |
| --- | --- | --- |
| `Prompt` | Template variables; message placeholders require `messages`. | Exactly `messages`. |
| `ChatModel` | Exactly `messages`. | `text`, `messages`, or both. |
| `Retriever` | `Query` is a string binding. | Exactly `documents`. |
| `Transform` | Operation-specific typed ports. | Operation-specific typed ports. |
| `Passthrough` | Exactly `value`. | Exactly `value` with the same type. |
| `Script` | Declared dictionary keys. | Declared dictionary keys. |
| `Lambda` | Descriptor-defined ports. | Descriptor-defined ports. |
| `Match` | Exactly one `text` string binding. | Exactly one `matches` list. |
| `Subgraph` | Child Graph inputs and State initialization fields. | Every child Graph output name. |
| `Race` | Common nested Graph inputs. | Common nested Graph output names. |
| `Batch` | `Items` is a list binding. | Exactly one ordered `items` list. |

Each Prompt message uses exactly one declaration form. A role message declares a non-empty `role` and `template`. A placeholder message declares a non-empty `placeholder` and may set `optional`; it cannot also declare `role` or `template`. The placeholder name must identify a Prompt input bound to `messages`.

Prompt, ChatModel, and Retriever components are added through Eino's native `AddChatTemplateNode`, `AddChatModelNode`, and `AddRetrieverNode` paths inside typed nested Graphs. Package-owned Transform, Script, Race, Batch, and State adapters use Eino Lambdas when no serializable native component contract exists.

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

### Audio turns

`Config.TranscribeInput` lets a host supply native Model transcription without changing the Graph definition. After push-to-talk audio EOS, the Transformer calls it, publishes the user's transcript under the same StreamID, and initializes ordinary text in `input.text` and `input.messages` before Memory recall and Graph execution. Failure or cancellation prevents Graph execution; an empty transcript ends the turn without a reply. Observe and History use that same text, and ordinary text turns bypass transcription. This callback and a Graph `AudioTranscript` receiver are mutually exclusive; the host chooses one input integration.

Hosts may also set an internal Graph receiver to send audio directly to a ChatModel, allowing Graph execution before transcription. GizClaw derives this integration only for simple Graphs that do not require current text; the persisted Workflow Schema exposes no receiver field. The following describes this Go API mode:

`ChatModelNode.AudioTranscript` makes one root Graph ChatModel node the transcriber of audio user turns; setting it in a nested Graph or on more than one node fails `New`. When the Graph has that node, an ordinary user `audio/*` route (never the `history.user_audio` sideband) starts a turn at its first audio chunk and completes it at EOS: a new audio route interrupts the previous turn, an `interrupted` EOS discards the route, and any other EOS error fails the session. Each Blob stays one audio part of the current user message, so the node must receive it through `input.messages`; `input.text` is empty for that turn, so a Memory recall that uses it as the query is skipped (see below). A Graph with neither that node nor `TranscribeInput` accepts text turns only.

For an audio turn, the Transformer first publishes a `history.user_audio` sideband under the audio input StreamID, so History orders the user entry before the reply. The ChatModel component the node calls reports the audio transcript anywhere in its reply stream as a stream message built with `TranscriptMessage`; how the transcript is obtained belongs to the component and the Model behind it (the GizClaw GenX adapter turns a Generator `genx.InputTranscriptLabel` chunk into that message). The node records the first reported transcript as the turn's user text for History and Memory observe, and publishes it as a `transcript`-labelled user text route under the same StreamID, the shape an ASR stage produces; transcripts reported again in Tool rounds are ignored. A turn without a reported transcript still publishes its reply with an empty user text, and History keeps only the user audio and the reply. History keeps no audio parts, so later turns do not resend the audio.

Memory recall runs before the model, when an audio turn has no transcript yet. A recall whose `QueryFrom` is `input.text` (`Config.Memory.Recall` and `MemoryRecallNode`) gets an empty query in an audio turn: it sets its output to an empty string without calling the Store, and the turn does not fail. A Graph that needs recall in audio turns can point `QueryFrom` at a string State field and derive the query from `history.messages` with a Script before the recall node, for example the current text in a text turn and the previous user text in an audio turn:

```python
def run(input):
    query = input["text"]
    if query == "":
        for message in input["history"]:
            if message["role"] == "user" and message["content"] != "":
                query = message["content"]
    return {"query": query}
```

The Script node binds `text: input.text` and `history: history.messages` and writes `query` to the field that `MemoryRecallNode.QueryFrom` references. Recall then follows the topic of the previous turn rather than the current utterance, and the first audio turn still recalls nothing because History is empty. When recall must use the current utterance, the host should supply `Config.TranscribeInput` or convert audio to a text turn with upstream ASR. GizClaw automatically prepares transcription for original Graphs with Memory or Scripts, without changing those Graphs.

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

```go
type ResolvedLambda struct {
    Lambda  *compose.Lambda
    Inputs  map[string]StateType
    Outputs map[string]StateType
}
```

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

Product Workflows select persisted fields with `state_persistence.fields`. Server configuration `services.agent_host.persistence.state_store` references a SQL Store: `graph_states` stores snapshots and `graph_state_scopes` retains deletion fences. Missing selected fields receive their declared typed zero value on first load. Reload restores only selected fields. Nested object/list integers retain signed 64-bit precision, and integral floating-point values retain their numeric type through optional snapshot type hints. New snapshots use format version 1; unversioned snapshots retain their previous JSON float64 decoding until a normal successful CAS write. Internal conversation History uses the mutable log referenced by `services.agent_host.persistence.history_store`.

Persistent State is optional:

```go
State: &eino.StatePersistenceConfig{
    Store:  stateStore,
    Scope:  "workspace/assistant",
    Fields: []string{"summary", "turn_count"},
}
```

The Store loads one versioned snapshot before the Graph. Only configured fields are validated and copied into fresh local State. Immediately before successful primary EOS, final configured fields are committed with compare-and-swap. Failure, cancellation, interruption, invalid State, or conflict does not overwrite the prior snapshot.

History uses an optional `logstore.MutableStore`, stable scope, Agent ID, ContextID, and bounded query limit. Without a Store, the Transformer keeps a bounded Agent-local History. Stored records contain ordered user and pull-visible assistant messages; interrupted assistant records contain an interruption marker.

Memory uses an optional provider-neutral `memory.Store`. Each Recall declaration runs before the Graph, renders ordered facts as `- text` lines, writes its string State field, and contributes to `memory.recalled`. A Recall whose query text is blank, as in an agent-initiative turn, skips the Store and writes an empty result instead of failing the run. Observe runs after delivery observation. It submits pull-visible turns and explicitly declared State fact bindings only.

When `WaitForCompletion` is true, the Store must implement `memory.OperationWaiter`, and primary EOS waits for terminal operation success. When false, Observe acceptance still precedes EOS; a Store implementing `memory.AsyncOperationProcessor` may process a pending operation asynchronously.

## Validation and errors

`New` rejects invalid names, node unions, State types, merges, ports, bindings, component schemas, output MIME, duplicate output ownership, unreachable nodes, nodes that cannot reach `end`, unknown routing targets, impossible fan-in, concurrent writers, invalid cycles, recursive nesting deeper than 16 levels, and partial Store configuration. Errors include the Graph path and offending node or field.

Runtime provider, Store, Script, component, cancellation, byte-limit, and optimistic-concurrency failures terminate every active route with an error EOS. No failed Graph run commits persistent State.

The Eino Transformer depends only on the GenX `ToolInvoker` interface and does not receive RuntimeProfile, Toolkit policy, resource, or executor-registry details. One root `Transform` invocation shares its call-ID set and `MaxToolCalls` budget across nested Graphs. Provider call IDs remain inside Eino and are associated with the raw JSON result returned by `InvokeTool`. Zero uses 32 and negative values are rejected. Independent invocations may execute the shared invoker concurrently and reuse provider call IDs; resolution, invocation, invalid-result JSON, cancellation, duplicate-ID, and exhaustion failures remain local to one invocation.

For agent initiative, ChatModel omits content-free user messages rendered by Prompt while preserving system instructions, history, and multimodal input.

## Realtime speech endpointing

Eino realtime speech input explicitly sets Volc ASR `end_window_size=200` and
`force_to_speech_time=1000`. The former shortens endpointing after speech while
the latter preserves the minimum speech duration. Push-to-Talk keeps the ASR
Builder defaults. The model still starts only after definite ASR text EOS;
interim transcripts do not trigger a dialogue turn.

With final-reply verification enabled, malformed model Tool JSON can request regeneration before dispatch. Syntax and reply corrections share the same two-correction budget. Invalid proposals enter neither invocation nor wire history; parameters are not patched and no result is fabricated. Persistent errors or disabled verification still fail closed.
