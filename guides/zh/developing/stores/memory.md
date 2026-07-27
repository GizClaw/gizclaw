# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) 是 Agent runtime 共用的 provider-neutral 长期记忆边界。Flowcraft、Mem0 和 Volc 适配器分别位于 `flowcraft`、`mem0`、`volc` 子包。

## 契约

每次操作都携带结构化 `Scope`。`AppID`、`UserID`、`AgentID` 和 `RunID` 是四个相互独立的可选路由维度；公共契约不定义 App→User→Agent→Run 层级，也不把 `RunID` 解释为通用 Conversation。空字段表示没有选择该维度，不表示 wildcard、继承或全局可见。

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
	Text:  "玩家偏好什么？",
	Limit: 10,
})
```

Adapter 只保留 native provider 能准确表达的维度和组合，不能保持时返回 `ErrUnsupported`：

| 公共字段 | Flowcraft Recall | Mem0 / Volc Memory |
| --- | --- | --- |
| `AppID` | `RuntimeID` | `app_id` |
| `UserID` | `UserID` | `user_id` |
| `AgentID` | `AgentID` | `agent_id` |
| `RunID` | 不支持 | `run_id` |

Flowcraft 要求非空 `AppID`，允许空 `UserID` 形成 runtime-global Memory，并保留可选 `AgentID`。Mem0/Volc 支持 App-only、User-only、Agent-only、Run-only scope，也支持这些独立维度的组合。Adapter 会发送所有已选择的维度，并在 recall 与变更校验中使用 `AND` filter。

`Text` 和 `Turns` 是待提取的原始材料；`Facts` 是上层已经结构化的候选事实。Provider 必须保持候选事实的文本与其支持的 attributes，无法直接写入时返回 `ErrUnsupported`，不能把候选事实静默送回模型二次提取。Flowcraft adapter 支持直接写入，并把 `kind`、`subject`、`predicate`、`object` 和 `entities` 映射到 native fact 字段。

`UpdateRequest`、`DeleteRequest` 和 `OperationRequest` 都必须重新携带调用方的 `Scope`，以及 Store 返回的不透明 fact、revision 或 operation locator。Locator 不是授权来源：Adapter 在 mutation 或完成异步操作前校验请求 Scope 与 locator、provider record 一致。原始 provider ID 不能绕过 App 边界。

异步 `Observe` 返回 operation。实现 `OperationWaiter` 的 store 使用调用方已有的 `context.Context` 等待，不在 constructor 中启动后台 goroutine。使用相同持久化依赖重新构造 Flowcraft adapter 后，仍可恢复 durable operation locator。

`memory.BindApp(store, appID)` 返回一个借用的 Store view。它只填充或校验 `Scope.AppID`，不生成、清空、拼接、hash 或改写调用方的 `UserID`、`AgentID` 和 `RunID`。冲突 AppID 返回 `ErrInvalidInput`。View 不拥有也不关闭底层 Store，并且只有在底层实现 `OperationWaiter`、`AsyncOperationProcessor` 或 `StatisticsProvider` 时才暴露相同 capability。

## Provider 构造

Provider 包只接收内存中的 runtime dependency，不解析 YAML、不展开环境变量、不读取配置文件，也不决定产品身份。

Flowcraft 只通过一个 `flowcraft.Config` 构造。该结构可注入 `ModelLoader`、retrieval index、temporal store、evidence store、async queue 和 side-effect outbox。注入的 dependency 仍由调用方拥有；没有注入时，adapter 使用 Flowcraft 的内存实现。

```go
store, err := flowcraft.New(ctx, flowcraft.Config{
	Loader:         loader,
	Extraction:     flowcraft.ExtractionConfig{Model: "extractor"},
	Embedding:      flowcraft.EmbeddingConfig{Model: "embedding"},
	RetrievalIndex: index,
	TemporalStore:  temporal,
})
```

Mem0 只通过一个 `mem0.Config` 构造。`FlavorPlatform` 使用 `Authorization: Token`，并将所有已选择的维度映射到对应的 `app_id`、`user_id`、`agent_id` 和 `run_id`。Mem0 OSS 不提供 `app_id`，因此 `FlavorSelfHosted` 会把完整四维 Scope 编码到一个保留的原生 `user_id` 中；配置 key 时使用 `X-API-Key`。这样既能精确保持 Workspace App 隔离，也不会改写调用方逻辑上的 User、Agent 或 Run 维度。Update/Delete 先读取 provider record 并校验完整编码 scope，再执行 ID mutation。

Volcengine AgentKit/Viking MEM0 只通过一个 `volc.Config` 构造。它接收显式的 Mem0 data-plane key 或 credential resolver，解析 credential 后复用 Mem0 adapter；data-plane endpoint 必填。

## MemoryLayout、RuntimeProfile 与 Workflow

Memory 不再是 Server Config 中的 `stores.kind: memory`。Portable policy、部署连接和 Graph 消费行为分属三个资源面：

- Admin `MemoryLayout` 同时声明 Flowcraft、Mem0 和 `volc_mem0` 的 provider policy，不包含 endpoint、API key、DSN 或目录。
- RuntimeProfile 的 `resources.memories.<alias>` 选择 Layout、实际 driver 和严格类型化 connection。Connection 中的 endpoint、API key、project ID、DSN 或目录直接属于该 RuntimeProfile，不引用 Credential 资源。
- Workflow 顶层 `memory` 只引用 RuntimeProfile alias。Graph 的 `memory_recall` / `memory_observe` node 决定何时读写、query 从哪里来、结果写到哪里，以及如何从 turn 或 state 构造 fact；这些映射不属于 MemoryLayout。

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata:
  name: pet-memory
spec:
  flowcraft:
    extraction:
      model: memory-extractor
      mode: two_pass
    embedding:
      model: text-embedding
    bbh:
      search_overfetch: 20
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

`MemoryLayout` 的三个 provider block 都必须存在。Flowcraft block 中的 extraction、embedding 和 rerank model 是 RuntimeProfile model alias；只有实际选择 `driver: flowcraft` 时才解析这些 alias。

```yaml
spec:
  resources:
    memories:
      pet-memory:
        layout_id: pet-memory
        driver: flowcraft
        connection:
          type: flowcraft_bbh
```

`flowcraft_bbh` 是不需要外部服务的 portable connection。它使用
`<server-workspace>/data/memory/<runtime-profile>/<memory-alias>`，Workspace 名作为 `Scope.AppID` 隔离数据。其他合法 connection 是 `flowcraft_object_store`（`directory`）、`flowcraft_postgresql`（`dsn`）、`mem0`（`project_id`、`endpoint`、`api_key`）和 `volc_mem0`（`memory_project_id`、`endpoint`、`api_key`）。Driver 与 connection type 必须匹配，未知字段、缺失 key 和无效 endpoint 会在 RuntimeProfile 写入或解析时被拒绝。

对于 Mem0 和火山云，Project ID 记录与所选数据面 API key 配套的部署/控制面身份。运行时 Fact 请求通过该 key 完成 Project 路由，不会再发送独立的 Project ID 字段。

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

同一 Workspace 的所有 stream 共用一个 Agent generation。数据可见性的稳定边界是同一 Workspace AppID、同一 memory driver 和同一 RuntimeProfile memory binding 指向的物理 connection。修改 extraction、recall、write、prompt、`top_k` 或 mode 不改变 canonical data；Flowcraft embedding、rerank 或 BBH policy 改变时，从 canonical facts 在 staging index 中重建，成功后原子发布，失败不会发布部分或混合索引。切换 driver 或 binding 可以切换物理数据源，不自动迁移或删除；切回仍存在的原 connection 后可以重新访问原数据。

## Ownership 与错误

Provider adapter 不关闭注入的 dependency。构造 workspace、index、HTTP client 或 credential dependency 的 composition root 拥有它们，并按构造顺序的逆序关闭资源。

稳定的 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 保留 `errors.Is` 语义。无法完整保持 filter、attribute patch 或 conditional-write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃条件。错误不得暴露 API key、access-key credential 或带 credential 的 response body。
