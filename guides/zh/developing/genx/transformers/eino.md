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
    ToolInvoker: runtimeTools,
    MaxToolCalls: 32,
    Limits:     eino.Limits{MaxOutputBytes: 4 << 20},
})
```

`New` 会校验并复制声明式配置，解析所有 component 与 named Lambda，构造 Eino 原生 node 和 routing，且只编译一次 root Graph 与各 nested Graph。构造过程不连接 provider、不启动常驻 worker，也不修改全局注册。

`ComponentResolver`、`LambdaResolver`、解析后的 component、Lambda 和 Store 都归调用方所有，并且必须支持并发使用。`Config` 不接受预构造 Agent、Runnable、可变 `compose.Graph`、Graph factory、raw graph callback、credential、provider endpoint 或产品 Resource。

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
| `input.messages` | `messages` | 有序 History 加当前 user message；Agent 主动开场只有 History，不追加空 user message。 |
| `input.parts` | `list` | defensive copy 后的非文本 input part。 |
| `history.messages` | `messages` | 仅包含此前的有序 History。 |
| `memory.recalled` | `string` | 合并后的 recall 渲染结果。 |
| `input.safety_fence` | `string` | 宿主通过 `Config.SafetyFence` 提供的安全围栏文案，未选择时为空字符串；batch、race 与子图继承同一值。Transformer 不会自行放置它，未绑定的 Graph 不受影响。 |
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
| `Match` | 仅一个 `text` string binding。 | 仅一个 `matches` list。 |
| `Subgraph` | Child Graph input 与 State initialization field。 | 全部 child Graph output name。 |
| `Race` | 所有 nested Graph 共有的 input。 | 所有 nested Graph 共有的 output name。 |
| `Batch` | `Items` 是 list binding。 | 一个保持顺序的 `items` list。 |

每条 Prompt message 必须且只能使用一种声明形式。角色消息同时声明非空 `role` 和 `template`；占位消息声明非空 `placeholder`，可以设置 `optional`，但不能同时声明 `role` 或 `template`。占位名称必须对应绑定到 `messages` 的 Prompt input。

Prompt、ChatModel 和 Retriever 通过 Eino 原生 `AddChatTemplateNode`、`AddChatModelNode`、`AddRetrieverNode` 路径加入 typed nested Graph。没有 serializable native component contract 的 Transform、Script、Race、Batch 与 State adapter 使用 Eino Lambda。

ChatModel 调用解析后的 Eino streaming interface。model node 直接拥有 declared text output 时，文本 chunk 会增量发布。配置 `ToolInvoker` 后，`ResolveTools` 取得的函数名、说明和 schema 会通过 Eino model option 传入；带关联 ID 的 ToolCall 按模型顺序通过 `InvokeTool(name, arguments)` 执行，native tool message 被追加后继续同一个 model node。内部 call/result 不公开输出；完成的 model turn 没有文本时，请求 `text` port 仍会失败。

### 音频 turn

`ChatModelNode.AudioTranscript` 让 root Graph 中的一个 ChatModel node 转写音频 user turn；在嵌套 Graph 中设置或由多个 node 设置都会在 `New` 失败。Graph 含该 node 时，普通 user `audio/*` route（不含 `history.user_audio` sideband）以第一个音频 chunk 开始、以 EOS 完成一轮：新的音频 route interrupt 上一轮，`interrupted` EOS 丢弃该 route，其他 EOS error 使 session 失败。每个 Blob 作为一个 audio part 留在当前 user message 中，因此该 node 必须通过 `input.messages` 收到它；该轮 `input.text` 为空，以它为 query 的 Memory recall 被跳过（见下文）。没有该 node 的 Graph 仍只接受文本 turn。

音频轮中，Transformer 先以音频 input 的 StreamID 发布 `history.user_audio` sideband，使 History 中的用户条目排在回复之前。该 node 调用的 ChatModel component 在回复流中任意位置用 `TranscriptMessage` 构造的 stream message 报告这段音频的 transcript；怎样得到 transcript 由 component 与其背后的 Model 负责（GizClaw 的 GenX 适配把 Generator 的 `genx.InputTranscriptLabel` chunk 转成该 message）。Node 把第一次报告的 transcript 作为本轮 user text 写入 History 与 Memory observe，并以同一 StreamID 发布 `transcript` label 的 user text route，形状与 ASR stage 相同；Tool round 重新报告的 transcript 被忽略。没有报告 transcript 的轮次照常发布回复，user text 为空，History 只保存用户音频和回复。History 不保存音频 part，后续轮次不再发送音频。

Memory recall 在模型运行之前执行，此时音频轮的 transcript 还不存在。`QueryFrom` 为 `input.text` 的 recall（`Config.Memory.Recall` 与 `MemoryRecallNode`）在音频轮得到空 query，把 output 置为空字符串且不调用 Store，不会使该轮失败。需要在音频轮召回的 Graph 可以把 `QueryFrom` 指向一个 string State field，并在 recall node 之前用 Script 从 `history.messages` 推导 query，例如文本轮使用当前文本，音频轮回退到上一条 user 文本：

```python
def run(input):
    query = input["text"]
    if query == "":
        for message in input["history"]:
            if message["role"] == "user" and message["content"] != "":
                query = message["content"]
    return {"query": query}
```

该 Script node 绑定 `text: input.text`、`history: history.messages`，把 `query` 输出到 `MemoryRecallNode.QueryFrom` 引用的 field。这样召回依据的是上一轮的话题而不是当前这句话；第一轮音频没有可用的 History，仍然不召回。必须按当前话语召回的场景应在 Transformer 之前用 ASR 把音频转成文本轮。

### Match

`MatchNode` 在 `New` 中编译共享的 `pkgs/genx/match` rules，通过 `ComponentResolver.ResolveChatModel` 只解析一次 model alias，并向模型发送一个 system message 和一个 user string。它不声明或执行 tools。

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
                "title": {Label: "歌曲名", Type: "string"},
            },
            Patterns: []match.Pattern{{Input: "我想听[title]"}},
        }},
    },
}
```

`text` 可以绑定 `input.text` 或声明为 string 的 State field；`matches` 必须指向已声明的 `StateList` field。空字符串是合法输入。Node 使用共享 Match parser 消费全部模型 chunk，只有 stream 完整成功后才写入 JSON-compatible 有序结果；model、stream、parse 或 cancellation error 都不会发布 partial State value。同一个编译后 node 可以并发运行，也可以用于 Subgraph、Race 或 Batch。

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

返回 dictionary 必须与声明的 output key 完全一致。支持 null、boolean、integer、有限 number、text、list、object、messages、documents 和 binary。Binary 在 Starlark 内使用 base64 text；message 使用 `{"role": "...", "content": "..."}`，多模态 message 的文本分段也通过 `parts` 提供；document 使用 `id`、`content` 和可选 `metadata`。

每个 Script limit 都必须为正数。step exhaustion、timeout、cancellation、malformed source、runtime error、byte-limit failure、unsupported conversion、缺失 output 或未声明 output 都会终止 Graph run。Sandbox 不提供 file、network、environment、process、random、Store、Tool、Graph 或 native Go access。

Starlark 提供 `json.encode` / `json.decode`、有界 RE2 `regex_find` / `regex_replace` 和 `now_millis()`。`regex_find` 的全局匹配和捕获数量最多为 4096，在构造结果前检查 output-byte 预算，并检查取消。`now_millis()` 返回当前 Unix 毫秒；日期与随机业务结果需要由场景显式保存为 State，才能在重载后保留。

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

`Compile.PrimaryOutputMode` 默认为 `fixed`，所有成功路径必须经过指定的 primary node。设置 `first_output` 时，本轮第一个实际发布的 output 成为 primary；只有实际执行的 output 才发出 BOS/EOS，保留配置的 Name 并使用 `assistant` label。空字符串是合法发布；没有发布的路径以错误结束，即使 State 中仍有旧值。其他已执行 output 先结束，primary EOS 最后结束。History 与 Memory 使用所有已交付 output 的文本，并等待这些 output 的交付边界。

语音输入经 Audio Dock 排除带 `StreamCtrl.TextInterim` 的中间假设，只聚合定稿文本内容。普通文本和一轮内多个定稿段仍按原顺序追加；仅中间结果的 interrupted route 不启动 Graph，也不写 user History。

每个已完成 text turn 都创建新的 output route 与 StreamID。每条 route 都有 BOS、data 和独立 EOS。Graph 仍在运行时 model text 可以增量到达。Non-primary route 按 name 稳定排序先结束；成功的 primary EOS 是最后一个边界。

Output buffer 不依赖 downstream pull，最多增长到 `Limits.MaxOutputBytes`；超限会使全部 route 失败。新的 text BOS 会 interrupt 上一轮，取消其 Graph 与 child，丢弃尚未 pull 的 suffix，并且只保留下游已观察到的 assistant prefix。

上游 text EOS 携带非空 StreamID 和精确错误 `interrupted` 时，只有当它匹配当前未完成 input route，或匹配被 replacement BOS 明确取代的 route，才表示 turn-scoped replacement terminal。Eino 对 active match 丢弃已缓冲的 text 和 part，并将已知 superseded route 的 terminal 作为 stale 忽略；未知或不匹配的 StreamID 仍保留原有校验错误。Session 最多跟踪 64 个等待 terminal 的 superseded route；超出边界时 session 失败，而不是让 replacement state 无界增长。合法 replacement 下 Transformer session 保持打开，只有 replacement BOS 负责 interrupt active Graph。其他非空 input terminal error 仍会使 session 失败。

除上述音频 turn 外，非文本 route 会原样 bypass。包含 blob 的 text turn 只有在 Graph 显式绑定 `input.parts` 时才接受，否则以 unsupported multimodal input 失败；如何解释这些 defensive copy 后的 part 由 component adapter 决定。

## State、History 与 Memory

产品 Workflow 可以通过 `state_persistence.fields` 选择持久化字段；Server 的 `services.agent_host.eino.state_store` 引用 SQL Store，状态保存在 `graph_states`，删除边界保存在 `graph_state_scopes`。首次加载缺失字段时按声明类型初始化零值；重载后只恢复选择的字段。Object/List 中的嵌套整数保留 signed 64-bit 精度，整值浮点数通过可选 snapshot 类型提示保留其 numeric type。内部对话 History 使用 `services.agent_host.eino.history_store` 的 mutable log。

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

Memory 使用可选的 provider-neutral `memory.Store`。每个 Recall 在 Graph 前执行，把有序 fact 渲染为 `- text` 行，写入声明的 string State field，并加入 `memory.recalled`。Recall 的 query 文本为空时（例如 Agent 主动开场 turn）跳过 Store，写入空结果而不是让本次 run 失败。Observe 在 delivery observation 后执行，只提交 pull-visible turn 与显式声明的 State fact binding。

显式 `memory_observe` node 可使用 `text_from` 与 `turns_from` 向 Store 提交原始抽取材料；这些 binding 与 direct `facts` 互斥。Direct Facts 不调用模型抽取。`memory_recall.filters` 传递 provider-neutral filter；`attributes.kind` / `attributes.lane` 可用于筛选业务记忆。Store 是否支持对应过滤语义由 provider contract 决定。

`WaitForCompletion=true` 时 Store 必须实现 `memory.OperationWaiter`，primary EOS 会等待 operation terminal success。设为 false 时，Observe acceptance 仍在 EOS 前完成；实现 `memory.AsyncOperationProcessor` 的 Store 可以异步处理 pending operation。

## Validation 与 error

`New` 会拒绝非法 name、node union、State type、merge、port、binding、component schema、output MIME、重复 output ownership、unreachable node、不能到达 `end` 的 node、未知 routing target、impossible fan-in、concurrent writer、非法 cycle、超过 16 层的递归 nesting 和不完整的 Store config。Error 会包含 Graph path 以及对应 node 或 field。

Provider、Store、Script、component、cancellation、byte limit 和 optimistic-concurrency runtime error 都会让所有 active route 以 error EOS 终止。失败的 Graph run 不提交 persistent State。

Eino Transformer 只依赖 GenX `ToolInvoker` interface，不接收 RuntimeProfile、Toolkit policy、resource 或 Executor registry 细节。一个 root `Transform` invocation 的 nested Graph 共用 call-ID set 与 `MaxToolCalls` budget；provider call ID 留在 Eino 内部，并与 `InvokeTool` 返回的 raw JSON result 关联。零值采用 32，负数非法。独立 invocation 可以并发执行同一 invoker 并复用 provider call ID；解析、执行、非法 result JSON、cancellation、重复 ID 和额度耗尽错误只影响当前 invocation。

Agent 主动开场时，ChatModel 会省略 Prompt 渲染出的无内容 user message，保留系统提示、历史以及多模态输入。

## 本地首响测量

Eino 的实时语音输入向 Volc ASR 显式传入 `end_window_size=200` 和
`force_to_speech_time=1000`。前者在语音结束后缩短断句等待，后者保留最短语音时长；
Push-to-Talk 仍使用 ASR Builder 的默认断句配置。模型仅在 ASR definite text EOS
之后运行，中间识别结果不会提前触发对话。

`go test ./pkgs/genx/transformers/eino -run '^TestRealtimeFirstResponseLatency$' -count=3 -v`
以可控 ASR、ChatModel 和 TTS 替身，运行 server ingress `RealtimeStream.Push` →
默认 80 ms 时间戳重排 → Audio Dock → Eino Prompt/ChatModel → Audio Dock 输出。
音频 route 保持打开，由 ASR 的 definite text EOS 启动模型；模型首块之后暂停，验证
首字和首音不依赖模型完成。测试分别报告重排、替身 endpoint/final、转交 Eino EOS、
Graph 前准备与 Prompt、首 token、text 转交、TTS 启动、输入与首包耗时。

此测试不连接 WebRTC、不测真实 Provider 或持久化 History Store。它使用与
`eino-concurrency-assistant` 相同的 Prompt → ChatModel 图形；该 Workflow 不启用
MemoryLayout/Recall，因此对应测量的 History Store 与 Recall 耗时为零。额外的
`history_and_memory` 对照配置分别注入 40 ms History query 与 60 ms Recall，单独报告
两个 Store 边界。配置 Memory 的图仍会在 Graph 前召回；不能为首响而跳过、截断
或将它移到模型之后。

本地结果不能代替 [Eino 首响验证](../../testing#eino-首响验证)。真实延迟排查应按
同一 input StreamID 对齐 ASR 的 text `stream_end`、Eino input `stream_end` 和 Eino
output `first_text`，再对齐 Audio Dock 的 `first_text`、`first_audio`；计时基准仍为
客户端 `speechEndedAt`。Interim transcript 的 `first_text` 不代表 ASR 已定稿，
也不能据此提前执行有副作用的对话轮次。

纯空白且不含附件的用户文字轮次不执行 Graph，也不写入模型对话；主动开场及带附件的输入仍按原有生命周期执行。
