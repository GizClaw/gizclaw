# Eino Transformer

[Graph 定义与校验](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/transformers/eino)

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

每个 output route 在 turn 启动时创建不可变的 `ResponseEpoch`，绑定该 turn 的输入
StreamID；`first_output` 延迟创建的 route 也使用预先建立的 epoch。正文和终止事件
携带同一个 epoch，因此消费端晚于 replacement input 才观察到首包时，仍能识别原始
输入归属。没有输入 route 的主动输出保持未归属状态。这些 metadata 仅在进程内传递。

## Graph contract

`GraphDefinition` 用于组合 State、node、路由和输出。

Binding 的 namespace 与类型由上述 Graph 定义统一维护。

Node input map 的 key 是 component input port；output map 的 key 是 node output port，value 是目标 State field。未知 binding、field、port 或类型不兼容都会在 `New` 失败。

State 支持 `string`、`boolean`、`integer`、`number`、`object`、`list`、`messages`、`documents` 和 `blob`。`replace` 适用于所有类型；`append` 只适用于 string、list、messages 和 documents；`object_merge` 只适用于 object，后写入的同名 key 会覆盖此前值。

同一 State field 的并发 writer 会被拒绝，除非 writer 之间存在明确的 Graph 顺序，或它们是同一个 `first_match` branch 的直接互斥 destination。原生 fan-out 应使用不同 State field，再通过显式 merge node 汇合。

## Routing 与 scheduling

Edge 映射到 Eino `AddEdge`。`first_match` 选择第一个命中的 route，否则使用 `Default`；`all_match` 选择所有命中的 route，仅在没有 route 命中时使用 `Default`。Predicate 支持递归 `all`、`any`、`not`，以及 exists、equal、contains 和数值比较。

`NodeTriggerAnyPredecessor` 使用 Eino any-predecessor scheduler。`NodeTriggerAllPredecessor` 形成 join barrier，并且只接受 acyclic Graph。存在 cycle 的 any-predecessor Graph 必须配置正数 `MaxRunSteps`。

`GraphCompileConfig.FanIn` 只映射 Eino `FanInMergeConfig.StreamMergeWithSourceEOF`。所列 node 必须存在且至少有两个 predecessor。State value 合并仍由 State merge policy 和显式 node 负责。

原生 parallelism 通过 Eino scheduler 执行 sibling path 并 join。`Race` 的语义不同：它执行隔离的 nested Graph，只保留一个 winner，取消 loser，并且只把 winner output 合并回 parent。所有选中工作都必须完成时使用 native fan-out；只允许一个结果生效时使用 `Race`。

## 支持的 node 与 port

每条 Prompt message 必须且只能使用一种声明形式。角色消息同时声明非空 `role` 和 `template`；占位消息声明非空 `placeholder`，可以设置 `optional`，但不能同时声明 `role` 或 `template`。占位名称必须对应绑定到 `messages` 的 Prompt input。

Prompt、ChatModel 和 Retriever 通过 Eino 原生 `AddChatTemplateNode`、`AddChatModelNode`、`AddRetrieverNode` 路径加入 typed nested Graph。没有 serializable native component contract 的 Transform、Script、Race、Batch 与 State adapter 使用 Eino Lambda。

`ChatModel.Temperature` 是可选配置；显式 `0` 会作为 Eino model option 传给已解析的 component，GizClaw 的 GenX bridge 保留这个零值。省略温度不覆盖 component 的默认设置。

ChatModel 调用解析后的 Eino streaming interface。model node 直接拥有 declared text output 时，文本 chunk 会增量发布。配置 `ToolInvoker` 后，`ResolveTools` 取得的函数名、说明和 schema 会通过 Eino model option 传入；带关联 ID 的 ToolCall 按模型顺序通过 `InvokeTool(name, arguments)` 执行，native tool message 被追加后继续同一个 model node。内部 call/result 不公开输出；完成的 model turn 没有文本时，请求 `text` port 仍会失败。

宿主可同时配置 `Config.VerifyToolResponse` callback，接收真实 `genx.ToolConversation`
快照和最终回复草稿。返回空反馈接受回复；非空反馈让原生模型最多纠正两次，不由
Transformer 生成工具名或参数。此配置要求 `ToolInvoker`，并暂存整个回复文本：中间
Tool round 与被拒绝的草稿不发布，通过检查的最终文本一次发布。callback 错误或
纠正耗尽使该轮以错误结束，不泄漏未校验文本。当前执行结果只位于 `ContinuationStart`
之后，历史成功不能充当本轮执行证明。调用关联、额度与已完成结果在纠正期间保留，
新的 invocation 独立持有快照；产品权限和语义仍由宿主检查。

Tool 结果快照通过内部调用 ID 与同名提议关联实际 arguments，随后移除 ID。
同名并行提议或结果顺序不同也不按名称猜目标；无匹配的结果不补参数。

### 结构化分流

解析后的组件实现 `StructuredChatModel` 时，ChatModel 可配置 `StructuredOutput`。
GizClaw 在 [Workflow source contract](https://github.com/GizClaw/gizclaw/blob/main/api/http/shared/workflows/eino.json)
中公开相同的 `chat_model.structured_output` 选项。节点通过已解析 GenX Generator 的原生
`Invoke` 能力传递同一份 typed messages、原音、History、模型参数和取消信号。
对象由 Provider JSON output 或原生 function calling 返回；节点不声明或执行 runtime Tools。
Provider 错误、截断或非法 JSON、schema 不符、组件不支持都会失败，不修补文本，也不回退到普通流式生成。
声明的 schema 原样传递，可选字段、nullability 和 additional-property 规则均不被改写；Provider 不支持时返回错误。
任何对象层级的重复键，包括数组内对象，都会在原始 JSON 写入 State 或发布前被拒绝。

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

完整对象通过已快照的 schema 和 `MaxOutputBytes` 检查后才能写入 State 或发布。
公开该节点时，校验后的 JSON 一次发布；内部控制节点不应列入 Graph outputs，只公开后续回复节点，
使分流结果不进入客户端文字、语音和对话 History。顺序节点可以复用已有 State 字段。
结构化节点与 `VerifyToolResponse` 不兼容，构造时直接拒绝，不会静默绕过该检查。
省略此选项时，普通回复仍增量流式输出，Tool continuation 行为不变。

### 音频 turn

宿主通过 `Config.AudioInput` 启用完整音频 turn。原始音频随 typed `input.messages` 进入 Graph。宿主提供 `InputText(ctx)` callback 时，声明的 `input.text` binding 使用节点 context 在 State 锁外等待同一输入的定稿转写；不依赖文字的回复路径可先流式输出。没有 callback 时，`input.text` 为空。ChatModel 只生成回复，不负责转写。没有启用音频 turn 时，Transformer 接收上游 ASR 的文本 turn。

音频 MIME EOS 启动当前轮；随后同一输入的成功纯控制 EOS 被消费。新音频 route 打断上一轮，`interrupted` EOS 丢弃该轮，其他错误沿 Stream 传播。每个 Blob 保留为当前 user message 的一个音频 part。音频 turn 中以 `input.text` 为 query 的 recall 跳过；有独立文本 query 的 recall 仍可执行。

ASR、用户 transcript 和用户原音记录由外层 [AudioDock](../agentkit#audio-dock) 与宿主 History 管理。Eino 不发布用户转写，也不额外发起模型转写请求。GizClaw 的 `spec.eino.audio_input` 选择 `auto`、`asr` 或 `model`。显式 `model` 通过 callback 支持带文本依赖函数的 PTT 图；省略或 `auto` 保留自动选择，连续 realtime 使用 ASR 文本路径。

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

`f_string` 系统提示中应转义字面大括号或使用不含大括号的说明；未转义的空大括号也参与模板插值。用户历史通过独立 message placeholder 注入，不能混入静态系统规则。

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

`MaxInputBytes` 对 messages 类型的 input binding 按 Script 实际可见的 `role`、`content` 和文本 `parts` 投影计量 JSON 字节；原音等未暴露给 Script 的消息字段不占用此节点的预算。投影不修改原始 Graph input，后续 Prompt placeholder 和 ChatModel 仍收到完整原音。普通文本和显式 binary input 继续受同一输入字节上限约束。

每个 Script limit 都必须为正数。step exhaustion、timeout、cancellation、malformed source、runtime error、byte-limit failure、unsupported conversion、缺失 output 或未声明 output 都会终止 Graph run。Sandbox 不提供 file、network、environment、process、random、Store、Tool、Graph 或 native Go access。

Starlark 提供 `json.encode` / `json.decode`、有界 RE2 `regex_find` / `regex_replace` 和 `now_millis()`。`regex_find` 的全局匹配和捕获数量最多为 4096，在构造结果前检查 output-byte 预算，并检查取消。`regex_replace` 同时按输出预算限制匹配索引与捕获数量，逐段检查取消和剩余字节后才追加替换内容；数字、命名捕获及 `$$` 保持 RE2 展开语义，超预算返回错误。`now_millis()` 返回当前 Unix 毫秒；日期与随机业务结果需要由场景显式保存为 State，才能在重载后保留。

## Named Lambda

Named Lambda 让 Go behavior 留在 serializable configuration 之外：

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

`Config.AudioInput` 保留原音于 `input.messages`。AudioDock 提供 `MessageChunk.InputText` 时，`input.text` 在实际读取的节点中按需等待同一输入的独立定稿转写；不读取它的回复路径可以先流式发布。绑定等待使用节点当前 context，不持有 State 锁，取消的 child 不会阻塞其他分支。命名 Lambda 也在独立的准备节点解析绑定，仅短暂访问 Eino 的框架 State，不在其 handler 内等待转写。转写不替换原音消息，也不创建第二个用户 turn。调用方未提供 callback 时，原音 turn 的 `input.text` 仍为空。

原音消息应通过 typed `input.messages` 或 Prompt placeholder 直达 ChatModel。Script 的消息视图是文本投影；不要把它的 messages 输出当成原音透传。函数只绑定需要的 `input.text`，回复模型仍绑定原音消息。

语音输入经 Audio Dock 排除带 `StreamCtrl.TextInterim` 的中间假设，只聚合定稿文本内容。普通文本和一轮内多个定稿段仍按原顺序追加；仅中间结果的 interrupted route 不启动 Graph，也不写 user History。

每个已完成 text turn 都创建新的 output route 与 StreamID。每条 route 都有 BOS、data 和独立 EOS。Graph 仍在运行时 model text 可以增量到达。Non-primary route 按 name 稳定排序先结束；成功的 primary EOS 是最后一个边界。

Output buffer 不依赖 downstream pull，最多增长到 `Limits.MaxOutputBytes`；超限会使全部 route 失败。新的 text BOS 会 interrupt 上一轮，取消其 Graph 与 child，丢弃尚未 pull 的 suffix，并且只保留下游已观察到的 assistant prefix。

上游 text EOS 携带非空 StreamID 和精确错误 `interrupted` 时，只有当它匹配当前未完成 input route，或匹配被 replacement BOS 明确取代的 route，才表示 turn-scoped replacement terminal。Eino 对 active match 丢弃已缓冲的 text 和 part，并将已知 superseded route 的 terminal 作为 stale 忽略；未知或不匹配的 StreamID 仍保留原有校验错误。Session 最多跟踪 64 个等待 terminal 的 superseded route；超出边界时 session 失败，而不是让 replacement state 无界增长。合法 replacement 下 Transformer session 保持打开，只有 replacement BOS 负责 interrupt active Graph。其他非空 input terminal error 仍会使 session 失败。

除上述音频 turn 外，非文本 route 会原样 bypass。包含 blob 的 text turn 只有在 Graph 显式绑定 `input.parts` 时才接受，否则以 unsupported multimodal input 失败；如何解释这些 defensive copy 后的 part 由 component adapter 决定。

## State、History 与 Memory

### 同一 turn 内连续讲述

`Config.ContinueFrom` 指向 root State 中使用 `replace` 的 boolean field。每次 Graph 成功执行后，true 请求同一用户 turn 内的下一次生成，false 结束播讲。每次执行前该字段重置为 false，Graph 必须主动写入 true；每次根 Graph 生成拥有独立的 `MaxRunSteps` 与 `MaxToolCalls` 预算及 call-ID set，播讲没有固定迭代次数。逐段搜索等工具调用不会累计耗尽整段播讲的默认 32 次额度。

此模式要求单一 `text/plain` string output，且其 State merge 为 `replace`；Graph 接收文本，音频可在进入 Graph 前转写。每次执行都必须发布新的非空白正文，包括将 `ContinueFrom` 设为 false 的最后一次；跳过输出或空白正文以 error EOS 终止，不提交本段 State。State 保留，`input.messages` 在原始输入后附加最近八次生成；内部续讲不创建用户消息。每轮正文使用独立的进程内 `StreamCtrl.MessageID`，`MessageEnd` 结束该正文的 MIME channel，播放 route 的 StreamID 与 BOS/EOS 保持连续。

配置外部 History reader 时，Host 每轮交付后保存一条权威 assistant History，并将该轮文字与音频对应为可重播记录；最初的用户消息只记录一次。Eino 只读这些记录，不追加第二份持久化副本。没有 reader 时，Eino 保留有界的本地 History。用户打断会取消当前生成、丢弃未交付后缀，并单独记录当前轮已交付的前缀；完成的旧段不合并、不重写。恢复或换主题建立新的用户 turn，仍按每轮生成分段保存。

Transformer 在文字交付及消息边界确认后才开始下一轮。产品 factory 对 Voice 启用 [Audio Dock 背压](../agentkit#连续讲述背压)，等待对应 TTS 音频交付，避免长期积压；该模式不支持 `speaker_voices`。每次交付的正文随后释放，续讲上下文保留最近八段，`Limits.MaxOutputBytes` 约束 Transformer 待交付缓冲。Memory observation 使用每段独立的 identity。

配置持久 State 时，每段完整交付、`MessageEnd` 确认及该段内部 Memory 处理和无 reader 时的本地 History 更新成功后，才对 `StatePersistenceConfig.Fields` 选择的字段执行 CAS；外部 History reader 模式下，Host 在转发交付确认前尝试写入唯一的持久化 History；该写入保持 best-effort，失败会记录日志，但不阻止 State CAS。成功返回的 version 用于下一段，最终 turn 收尾不重复 CAS。缓存队列、已播游标等字段因此可以在持续播讲中形成检查点。中断、生成失败或未确认消息边界的当前段不提交这些字段，恢复时加载最后一个成功检查点。CAS 冲突以 error EOS 终止，保留并发写入者的状态及此前完成的检查点。Host 成功写入 History 时，该写入先于 State CAS，但两者不是跨 Store 原子事务；CAS 失败前已经保存的 History 不回滚。

产品 Workflow 用 `spec.eino.conversation.continue_from` 选择该 State field。Graph 应生成短小、自然承接且以完整句子结束的正文；不需要额外的“继续”用户 turn。

产品 Workflow 可以通过 `state_persistence.fields` 选择持久化字段；Server 的 `services.agent_host.persistence.state_store` 引用 SQL Store，状态保存在 `graph_states`，删除边界保存在 `graph_state_scopes`。首次加载缺失字段时按声明类型初始化零值；重载后只恢复选择的字段。Object/List 中的嵌套整数保留 signed 64-bit 精度，整值浮点数通过可选 snapshot 类型提示保留其 numeric type。 新快照使用 format version 1；无版本快照保持原有 JSON float64 解码，直到一次正常成功的 CAS 写入。对话 History 复用 Workspace 的 `memory.HistoryStore`，Eino 只读。

Persistent State 是可选能力：

```go
State: &eino.StatePersistenceConfig{
    Store:  stateStore,
    Scope:  "workspace/assistant",
    Fields: []string{"summary", "turn_count"},
}
```

Store 在 Graph 前加载 versioned snapshot。只有配置列出的 field 会被校验并复制进新的 local State。普通 turn 在成功 primary EOS 之前用 compare-and-swap 提交最终 field；连续生成按上面的每段交付边界提交。Failure、cancellation、interruption、invalid State 或 conflict 不提交当前失败段，也不覆盖已完成段的检查点。

History 接收可选的 `memory.HistoryReader`，通过 `Scope` 选择会话，并以有界 limit 读取近期的 user/assistant 文本。外部 Reader 模式不调用 Append，也不保留第二份持久化历史；宿主负责完整的原始文本输入、ASR transcript 和已交付回复的记录，以及下一轮读取前的写入完成屏障。`ExcludeTurnID` 排除当前输入，避免历史与当前 user message 重复。没有 Reader 时使用有界 Agent-local History。

Memory 使用可选的 provider-neutral `memory.Store`。每个 Recall 在 Graph 前执行，把有序 fact 渲染为 `- text` 行，写入声明的 string State field，并加入 `memory.recalled`。Recall 的 query 文本为空时（例如 Agent 主动开场 turn）跳过 Store，写入空结果而不是让本次 run 失败。Observe 在 delivery observation 后执行，只提交 pull-visible turn 与显式声明的 State fact binding。

显式 `memory_observe` node 可使用 `text_from` 与 `turns_from` 向 Store 提交原始抽取材料；这些 binding 与 direct `facts` 互斥。Direct Facts 不调用模型抽取。`memory_recall.filters` 传递 provider-neutral filter；`attributes.kind` / `attributes.lane` 可用于筛选业务记忆。Store 是否支持对应过滤语义由 provider contract 决定。

`WaitForCompletion=true` 时 Store 必须实现 `memory.OperationWaiter`，primary EOS 会等待 operation terminal success。设为 false 时，Observe acceptance 仍在 EOS 前完成；实现 `memory.AsyncOperationProcessor` 的 Store 可以异步处理 pending operation。

## Validation 与 error

`New` 会拒绝非法 name、node union、State type、merge、port、binding、component schema、output MIME、重复 output ownership、unreachable node、不能到达 `end` 的 node、未知 routing target、impossible fan-in、concurrent writer、非法 cycle、超过 16 层的递归 nesting 和不完整的 Store config。Error 会包含 Graph path 以及对应 node 或 field。

Provider、Store、Script、component、cancellation、byte limit 和 optimistic-concurrency runtime error 都会让所有 active route 以 error EOS 终止。失败的普通 turn 或连续生成中的失败段不提交 persistent State；此前完整交付并提交的连续段保留。

Eino Transformer 只依赖 GenX `ToolInvoker` interface，不接收 RuntimeProfile、Toolkit policy、resource 或 Executor registry 细节。一次根 Graph 生成中的所有 ChatModel、模型工具续跑和 nested Graph 共用 call-ID set 与 `MaxToolCalls` budget；provider call ID 留在 Eino 内部，并与 `InvokeTool` 返回的 raw JSON result 关联。零值采用 32，负数非法。`ContinueFrom` 的下一次根 Graph 生成取得新的有界额度与 call-ID set，因此可以复用前一段的 provider call ID；同一次生成内重复 ID 或超额调用仍在进入 `ToolInvoker` 前失败。若调用 context 已携带上层工具作用域，则继续共用其额度与 ID，不通过子 Transformer 或内部续讲绕过上层限制。独立 invocation 可以并发执行同一 invoker；解析、执行、非法 result JSON、cancellation、重复 ID 和额度耗尽错误只影响当前 invocation。

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

启用最终回复校验时，模型 Tool 参数非法 JSON 可在发送设备前请求主模型重新生成，语法与回复纠正共用最多两次的预算。非法候选不进入调用或 wire history，不修补参数，不伪造执行结果；持续非法或未启用校验时仍以错误结束。
