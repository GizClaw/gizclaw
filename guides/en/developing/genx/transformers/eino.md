# Eino Transformer

`pkgs/genx/transformers/eino` builds a typed Eino Graph and exposes it as a concurrently reusable `genx.Transformer`. The package owns Graph construction, state, streaming, History, Memory, and turn lifecycle. It depends on GenX, Eino core, Starlark, and generic Stores; it does not import GizClaw Workspace, Workflow, Resource, AgentHost, or Toolkit types.

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
    Limits:     eino.Limits{MaxOutputBytes: 4 << 20},
})
```

`New` validates and copies the declarative configuration, resolves every referenced component and named Lambda, constructs native Eino nodes and routing, compiles the root Graph and every nested Graph once, and returns an immutable Transformer. It does not connect to a provider, start a permanent worker, or mutate global registration.

`ComponentResolver`, `LambdaResolver`, resolved components, and Stores remain caller-owned and must be safe for concurrent use. `Config` never accepts a preconstructed Agent, Runnable, mutable `compose.Graph`, Graph factory, raw callback, credential, provider endpoint, or product Resource.

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
| `input.messages` | `messages` | Ordered History followed by the current user message. |
| `input.parts` | `list` | Defensively copied non-text input parts. |
| `history.messages` | `messages` | Ordered prior History only. |
| `memory.recalled` | `string` | Combined rendered recall results. |
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
| `Subgraph` | Child Graph inputs and State initialization fields. | Every child Graph output name. |
| `Race` | Common nested Graph inputs. | Common nested Graph output names. |
| `Batch` | `Items` is a list binding. | Exactly one ordered `items` list. |

Prompt, ChatModel, and Retriever components are added through Eino's native `AddChatTemplateNode`, `AddChatModelNode`, and `AddRetrieverNode` paths inside typed nested Graphs. Package-owned Transform, Script, Race, Batch, and State adapters use Eino Lambdas when no serializable native component contract exists.

ChatModel uses the resolved Eino streaming interface. Text chunks are published incrementally when the model node owns a declared text output. ToolCalls are rejected. A requested text port fails when the model response contains no text.

The built-in Transform operations are:

- `select`: one `value` input and output with the same type;
- `concat_text`: string inputs listed exactly once in `Order`, optional `Separator`, and one `text` output;
- `decode_json`: one `text` input, one `object` output, positive byte limits, UTF-8 object JSON, and duplicate-key rejection; and
- `build_messages`: ordered system, user, or assistant items using either literal text or declared string input, and one `messages` output.

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

The return value must contain exactly the declared output keys. Supported values are null, boolean, integer, finite number, text, list, object, messages, documents, and binary. Binary values use base64 text inside Starlark. Messages use `{"role": "...", "content": "..."}` objects. Documents use `id`, `content`, and optional `metadata`.

Every Script limit must be positive. Step exhaustion, timeout, cancellation, malformed source, runtime failure, byte-limit failure, unsupported conversion, missing output, or undeclared output terminates the Graph run. The sandbox has no file, network, environment, process, clock, random, Store, Tool, Graph, or native Go access.

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

Non-text routes bypass the Transformer unchanged. A text turn containing blobs is accepted only when the Graph explicitly binds `input.parts`; otherwise it fails as unsupported multimodal input. Component-specific interpretation of those copied parts remains outside the package.

## State, History, and Memory

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

Memory uses an optional provider-neutral `memory.Store`. Each Recall declaration runs before the Graph, renders ordered facts as `- text` lines, writes its string State field, and contributes to `memory.recalled`. Observe runs after delivery observation. It submits pull-visible turns and explicitly declared State fact bindings only.

When `WaitForCompletion` is true, the Store must implement `memory.OperationWaiter`, and primary EOS waits for terminal operation success. When false, Observe acceptance still precedes EOS; a Store implementing `memory.AsyncOperationProcessor` may process a pending operation asynchronously.

## Validation and errors

`New` rejects invalid names, node unions, State types, merges, ports, bindings, component schemas, output MIME, duplicate output ownership, unreachable nodes, nodes that cannot reach `end`, unknown routing targets, impossible fan-in, concurrent writers, invalid cycles, recursive nesting deeper than 16 levels, and partial Store configuration. Errors include the Graph path and offending node or field.

Runtime provider, Store, Script, component, cancellation, byte-limit, and optimistic-concurrency failures terminate every active route with an error EOS. No failed Graph run commits persistent State.

The Eino Transformer does not define a Toolkit, advertise tools, execute ToolCalls, or continue a model after tool results.
