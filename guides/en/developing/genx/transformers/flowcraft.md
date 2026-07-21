# Flowcraft Transformer

`pkgs/genx/transformers/flowcraft` wraps a Flowcraft Graph as a concurrently reusable `genx.Transformer`. It depends only on GenX and generic Stores, not on GizClaw Workspace, Workflow, AgentHost, Claw, or Toolkit.

## Construction

```go
transformer, err := flowcraft.New(flowcraft.Config{
    ID:          "assistant",
    Name:        "Assistant",
    Description: "General assistant",
    Graph:       graphDefinition,
    MaxIterations: 32,
    PublishNodes: []string{"answer"},
    Models:       runtimeGenerator,

    History: historyLogStore,
    Memory:  longTermMemoryStore,
    State:   boardStateStore,

    MemoryScope: "runtime/user/assistant",
    RecallProfiles: []flowcraft.MemoryRecallProfile{{
        BoardVariable: "relevant_memory",
        Limit:         8,
    }},
    ObserveEnabled:           true,
    ObserveWaitForCompletion: false,
})
```

`Graph` is required and accepts only LLM nodes and script nodes with inline `source`. Scripts can operate on the Board and use runtime features such as match, but no Workspace is installed, so filesystem APIs are unavailable. `PublishNodes` explicitly selects which node assistant tokens can enter the output Stream.

An LLM node's `model` field is an alias such as `chat`. The Transformer resolves it internally as `Models.GenerateStream(ctx, "model/chat", modelContext)`; a Graph cannot supply a raw provider model ID or bypass the runtime alias.

The model adapter carries the GenX-defined max tokens, temperature, top-p, top-k, penalties, thinking, and extra fields. Flowcraft stop words, structured/image output, and ToolCall have no matching generic GenX text contract, so they return an explicit error instead of applying provider-specific guesses.

Parallel Graph execution always uses the Flowcraft SDK defaults: up to 10 branches, three nesting levels, and `last_wins` merge. A Graph without a fork creates no extra branches. The Publisher buffers speculative candidates, exposes only the accepted branch, and drops cancelled branches.

## Stream lifecycle

Each `Transform(ctx, input)` creates an internal ContextID. It correlates ordered turns only within that call and is not accepted by the public API, returned, persisted for resumption, or reused across Transform calls or process restarts. The same Transformer supports concurrent Transform calls with independent ContextIDs, runs, input accumulators, and output buffers.

Text input starts at BOS and is accumulated until the matching text EOS before the Graph runs. Each completed text turn produces a fresh output StreamID, BOS, streaming text, and EOS. Non-text content bypasses Flowcraft unchanged on its original route.

A new text BOS cancels an unfinished prior turn, discards its unfinished input and unpulled output, and emits an EOS with an `interrupted` error after that turn reaches its persistence boundary. Because a control-only BOS has no MIME declaration, Flowcraft treats it eagerly as the next text turn; a non-text route that must not interrupt uses a MIME-bearing BOS. History and Memory contain only assistant text that crossed the final delivery observation boundary; a discarded or not-yet-delivered suffix is not recorded. An interrupted user/assistant History pair is stored only when at least one assistant text delta was delivered, and its assistant message carries an interruption data marker; marker-only empty assistant messages are not stored.

## Store boundaries

- `History` uses the caller-provided `logstore.MutableStore` for ordered turns inside one Transform. An invocation-local in-memory transcript is used when it is nil.
- `State` uses a caller-prefixed `kv.Store` for JSON-serializable Board variables. `response`, `usage`, `tool`, `tmp_*`, and `__*` variables are excluded.
- `Memory` uses the provider-neutral `memory.Store`. Every recall and observation uses the fixed caller-configured `MemoryScope`.

`RecallRenderer` and `ObservationBuilder` have package defaults and may be replaced. The default recall value is a `Relevant memory:` list. The default observation contains the user turn and the assistant text actually pulled by downstream, never Board variables as facts.

With `ObserveWaitForCompletion=false`, EOS and the next turn wait for `Observe` acceptance but not for an asynchronous operation to materialize; later failure belongs to the Memory Store. When true, Memory must implement `memory.OperationWaiter`, and both the current EOS and next Graph turn wait for operation completion. The input pump continues reading in either mode and does not use downstream output as backpressure.

Toolkit continuation is outside this Transformer's current contract.
