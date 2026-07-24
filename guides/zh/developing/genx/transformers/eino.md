# Eino Transformer

`pkgs/genx/transformers/eino` 将 typed Eino Graph 封装为可并发复用的 `genx.Transformer`。该 package 自己负责 Graph 构造、State、streaming、History、Memory 与 turn lifecycle；它只依赖 GenX、Eino core、Starlark 和通用 Store，不导入 GizClaw Workspace、Workflow、Resource、AgentHost 或产品层 Toolkit 类型。

## 构造与所有权

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
    Toolkit:    executableToolkit,
    MaxToolCalls: 32,
    Limits:     eino.Limits{MaxOutputBytes: 4 << 20},
})
```

`New` 会校验并复制声明式配置，解析所有 component 与 named Lambda，构造 Eino 原生 node 和 routing，且只编译一次 root Graph 与各 nested Graph。构造过程不连接 provider、不启动常驻 worker，也不修改全局注册。

`ComponentResolver`、`LambdaResolver`、解析后的 component、Lambda 和 Store 都归调用方所有，并且必须支持并发使用。`Config` 不接受预构造 Agent、Runnable、可变 `compose.Graph`、Graph factory、raw callback、credential、provider endpoint 或产品 Resource。

`Agent.ContextID` 留空时，`New` 生成一个 Transformer 生命周期内稳定的 opaque identity。每个 turn 仍拥有独立的 invocation、run 和 output Stream identity。

## Graph contract

`GraphDefinition` 显式声明 State field、typed node、edge、branch、compile 行为和 output：

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

Binding 只接受以下 namespace：

| Binding | 类型 | 值 |
| --- | --- | --- |
| `input.text` | `string` | 已完成的 user text turn。 |
| `input.messages` | `messages` | 有序 History 加当前 user message。 |
| `input.parts` | `list` | defensive copy 后的非文本 input part。 |
| `history.messages` | `messages` | 仅包含此前的有序 History。 |
| `memory.recalled` | `string` | 合并后的 recall 渲染结果。 |
| State field 裸名称 | 声明类型 | 当前 invocation-local State value。 |

Node input map 的 key 是 component input port；output map 的 key 是 node output port，value 是目标 State field。未知 binding、field、port 或类型不兼容都会在 `New` 失败。

State 支持 `string`、`boolean`、`integer`、`number`、`object`、`list`、`messages`、`documents` 和 `blob`。`replace` 适用于所有类型；`append` 只适用于 string、list、messages 和 documents；`object_merge` 只适用于 object，后写入的同名 key 会覆盖此前值。

同一 State field 的并发 writer 会被拒绝，除非 writer 之间存在明确的 Graph 顺序，或它们是同一个 `first_match` branch 的直接互斥 destination。原生 fan-out 应使用不同 State field，再通过显式 merge node 汇合。

## Routing 与 scheduling

Edge 映射到 Eino `AddEdge`。`first_match` 选择第一个命中的 route，否则使用 `Default`；`all_match` 选择所有命中的 route，仅在没有 route 命中时使用 `Default`。Predicate 支持递归 `all`、`any`、`not`，以及 exists、equal、contains 和数值比较。

`NodeTriggerAnyPredecessor` 使用 Eino any-predecessor scheduler。`NodeTriggerAllPredecessor` 形成 join barrier，并且只接受 acyclic Graph。存在 cycle 的 any-predecessor Graph 必须配置正数 `MaxRunSteps`。

`GraphCompileConfig.FanIn` 只映射 Eino `FanInMergeConfig.StreamMergeWithSourceEOF`。所列 node 必须存在且至少有两个 predecessor。State value 合并仍由 State merge policy 和显式 node 负责。

原生 parallelism 通过 Eino scheduler 执行 sibling path 并 join。`Race` 的语义不同：它执行隔离的 nested Graph，只保留一个 winner，取消 loser，并且只把 winner output 合并回 parent。所有选中工作都必须完成时使用 native fan-out；只允许一个结果生效时使用 `Race`。

## 支持的 node 与 port

| Node | Input | Output |
| --- | --- | --- |
| `Prompt` | Template variable；message placeholder 必须绑定 `messages`。 | 仅 `messages`。 |
| `ChatModel` | 仅 `messages`。 | `text`、`messages` 或两者。 |
| `Retriever` | `Query` 是 string binding。 | 仅 `documents`。 |
| `Transform` | Operation-specific typed port。 | Operation-specific typed port。 |
| `Passthrough` | 仅 `value`。 | 仅同类型 `value`。 |
| `Script` | 声明的 dictionary key。 | 声明的 dictionary key。 |
| `Lambda` | Descriptor 声明的 port。 | Descriptor 声明的 port。 |
| `Subgraph` | Child Graph input 与 State initialization field。 | 全部 child Graph output name。 |
| `Race` | 所有 nested Graph 共有的 input。 | 所有 nested Graph 共有的 output name。 |
| `Batch` | `Items` 是 list binding。 | 一个保持顺序的 `items` list。 |

Prompt、ChatModel 和 Retriever 通过 Eino 原生 `AddChatTemplateNode`、`AddChatModelNode`、`AddRetrieverNode` 路径加入 typed nested Graph。没有 serializable native component contract 的 Transform、Script、Race、Batch 与 State adapter 使用 Eino Lambda。

ChatModel 调用解析后的 Eino streaming interface。model node 直接拥有 declared text output 时，文本 chunk 会增量发布。配置 Toolkit 后，它的 schema 会通过 Eino model option 传入；带关联 ID 的 ToolCall 按模型顺序执行，native tool message 被追加后继续同一个 model node。内部 call/result 不公开输出；完成的 model turn 没有文本时，请求 `text` port 仍会失败。

内置 Transform operation：

- `select`：一个 `value` input 与同类型 output；
- `concat_text`：`Order` 精确列出 string input，可配置 `Separator`，输出一个 `text`；
- `decode_json`：一个 `text` input、一个 `object` output、正数 byte limit、UTF-8 object JSON 和 duplicate-key rejection；
- `build_messages`：按顺序使用 system、user、assistant literal 或 string input，输出一个 `messages`。

## Starlark Script

Script source 在 `New` 中只 compile 一次并校验 initialization。每次 run 都会在配置的 step、timeout 与 cancellation 限制内初始化独立 module globals，再调用 entrypoint，因此 mutable global 不会跨 turn 泄漏。默认 entrypoint 是 `run`，接收一个 frozen dictionary：

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

返回 dictionary 必须与声明的 output key 完全一致。支持 null、boolean、integer、有限 number、text、list、object、messages、documents 和 binary。Binary 在 Starlark 内使用 base64 text；message 使用 `{"role": "...", "content": "..."}`；document 使用 `id`、`content` 和可选 `metadata`。

每个 Script limit 都必须为正数。step exhaustion、timeout、cancellation、malformed source、runtime error、byte-limit failure、unsupported conversion、缺失 output 或未声明 output 都会终止 Graph run。Sandbox 不提供 file、network、environment、process、clock、random、Store、Tool、Graph 或 native Go access。

## Named Lambda

Named Lambda 让 Go behavior 留在 serializable configuration 之外：

```go
type ResolvedLambda struct {
    Lambda  *compose.Lambda
    Inputs  map[string]StateType
    Outputs map[string]StateType
}
```

Resolver 在 `New` 执行。Descriptor 必须与全部 configured port 和 State type 匹配。Lambda ABI 是 `map[string]any` input 到 `map[string]any` output；应使用 `compose.InvokableLambda` 或同 shape 的 Eino Lambda。返回值必须包含所有 declared output port。Lambda 归调用方所有且必须支持并发调用。

## Race、Batch 与 Subgraph

Race branch 是具有相同 output schema 的完整 nested Graph：

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

Winner mode 包括 `first_output`、`first_success` 和 `predicate`。`first_output` 在第一个 declared output 发出时立即选定 winner 并取消 loser branch context，同时允许 winner 完成自己的 output。Predicate mode 对每个已完成 child State 计算 predicate。Child State 与 output buffer 相互隔离；winner 的 named Graph output 会复制到 parent。

Batch 对有序 list 执行 nested Graph：

```go
eino.BatchNode{
    Items:          eino.Binding{From: "documents"},
    Graph:          itemGraph,
    MaxConcurrency: 4,
}
```

每个 item 初始化 child 中名为 `item` 的 State field。执行有并发上限并且 fail-fast；结果 `items` 保持原输入顺序，错误时不返回 partial list。

Subgraph 只执行一次 nested Graph。名为 `text`、`messages`、`parts` 的 input 初始化对应 child input namespace，其他 input name 初始化同名 child State field。全部 nested Graph 都在 `New` 中校验并编译。

## Output 与 Stream lifecycle

`Outputs` 是唯一的 publication allow-list。每项指定一个 node 产生的 string 或 blob State field、route name 与 MIME type。Name 和 node-field source 必须唯一，并且恰好有一个 primary output。

每个已完成 text turn 都创建新的 output route 与 StreamID。每条 route 都有 BOS、data 和独立 EOS。Graph 仍在运行时 model text 可以增量到达。Non-primary route 按 name 稳定排序先结束；成功的 primary EOS 是最后一个边界。

Output buffer 不依赖 downstream pull，最多增长到 `Limits.MaxOutputBytes`；超限会使全部 route 失败。新的 text BOS 会 interrupt 上一轮，取消其 Graph 与 child，丢弃尚未 pull 的 suffix，并且只保留下游已观察到的 assistant prefix。

非文本 route 会原样 bypass。包含 blob 的 text turn 只有在 Graph 显式绑定 `input.parts` 时才接受，否则以 unsupported multimodal input 失败；如何解释这些 defensive copy 后的 part 由 component adapter 决定。

## State、History 与 Memory

Persistent State 是可选能力：

```go
State: &eino.StatePersistenceConfig{
    Store:  stateStore,
    Scope:  "workspace/assistant",
    Fields: []string{"summary", "turn_count"},
}
```

Store 在 Graph 前加载 versioned snapshot。只有配置列出的 field 会被校验并复制进新的 local State。成功 primary EOS 之前立即用 compare-and-swap 提交最终 field。Failure、cancellation、interruption、invalid State 或 conflict 都不会覆盖此前 snapshot。

History 使用可选的 `logstore.MutableStore`、稳定 scope、Agent ID、ContextID 和有界 query limit。没有 Store 时，Transformer 使用有界的 Agent-local History。Record 按顺序保存 user 与真正 pull-visible 的 assistant message；被中断的 assistant record 带 interruption marker。

Memory 使用可选的 provider-neutral `memory.Store`。每个 Recall 在 Graph 前执行，把有序 fact 渲染为 `- text` 行，写入声明的 string State field，并加入 `memory.recalled`。Observe 在 delivery observation 后执行，只提交 pull-visible turn 与显式声明的 State fact binding。

`WaitForCompletion=true` 时 Store 必须实现 `memory.OperationWaiter`，primary EOS 会等待 operation terminal success。设为 false 时，Observe acceptance 仍在 EOS 前完成；实现 `memory.AsyncOperationProcessor` 的 Store 可以异步处理 pending operation。

## Validation 与 error

`New` 会拒绝非法 name、node union、State type、merge、port、binding、component schema、output MIME、重复 output ownership、unreachable node、不能到达 `end` 的 node、未知 routing target、impossible fan-in、concurrent writer、非法 cycle、超过 16 层的递归 nesting 和不完整的 Store config。Error 会包含 Graph path 以及对应 node 或 field。

Provider、Store、Script、component、cancellation、byte limit 和 optimistic-concurrency runtime error 都会让所有 active route 以 error EOS 终止。失败的 Graph run 不提交 persistent State。

Eino Transformer 消费但不重新定义 GenX Toolkit。一个 root `Transform` invocation 的 nested Graph 共用 call-ID set 与 `MaxToolCalls` budget；零值采用 32，负数非法。独立 invocation 可以并发执行同一 Toolkit 并复用 provider call ID；executor、validation、serialization、cancellation、重复 ID 和额度耗尽错误只影响当前 invocation。
