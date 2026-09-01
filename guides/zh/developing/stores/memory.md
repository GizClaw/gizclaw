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

`Text` 和 `Turns` 是待提取的原始材料；`Facts` 是上层已经结构化的候选事实。Provider 必须保持候选事实的文本与其支持的 attributes，无法直接写入时返回 `ErrUnsupported`，不能把候选事实静默送回模型二次提取。Flowcraft、Mem0 和 Volc adapter 都支持 direct Fact；Flowcraft 把 `kind`、`subject`、`predicate`、`object` 和 `entities` 映射到 native fact 字段，Mem0/Volc 使用 `infer=false` direct import。

对于包含 direct Facts 的 Observation，非空 `Observation.ID` 是完整 `Scope` 内的幂等键。相同 ID 和相同 canonical direct-Fact payload 的并发调用或重试返回原 logical Fact 或 durable operation；Fact 文本、attributes 或 `ObservedAt` 改变时返回 `ErrConflict`。Adapter 在 native record 中保存 payload digest，并在提交前先对账；因此 provider 已接受但 response 丢失后的重试不会创建第二个 logical Fact。返回的 `Fact.Sources` 保留 `ObservationID`。这些 provider-owned metadata 不会暴露为业务 attributes。模型 extraction 的 provider-native dedup 行为不属于这个 direct-Fact 保证。

`UpdateRequest`、`DeleteRequest` 和 `OperationRequest` 都必须重新携带调用方的 `Scope`，以及 Store 返回的不透明 fact、revision 或 operation locator。Locator 不是授权来源：Adapter 在 mutation 或完成异步操作前校验请求 Scope 与 locator、provider record 一致。原始 provider ID 不能绕过 App 边界。

异步 `Observe` 返回 operation。实现 `OperationWaiter` 的 store 使用调用方已有的 `context.Context` 等待，不在 constructor 中启动后台 goroutine。Flowcraft constructor 不枚举 durable scopes，也不读取 canonical facts 来预热 operation cache。使用相同持久化依赖重新构造 adapter 后，`Wait()` 会先解码 locator 并校验调用方的完整 `Scope`，再只从 locator 对应的 scope 恢复 durable operation；scope 不匹配时在读取 temporal store 前返回 `ErrInvalidInput`。

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

Mem0 只通过一个 `mem0.Config` 构造。`FlavorPlatform` 使用 `Authorization: Token`，并将所有已选择的维度映射到对应的 `app_id`、`user_id`、`agent_id` 和 `run_id`。Mem0 OSS 不提供 `app_id`，因此 `FlavorSelfHosted` 会把完整四维 Scope 编码到一个保留的原生 `user_id` 中；配置 key 时使用 `X-API-Key`。这样既能精确保持 Workspace App 隔离，也不会改写调用方逻辑上的 User、Agent 或 Run 维度。Update/Delete 先读取 provider record 并校验完整编码 scope，再执行 ID mutation。Direct import 当前一次接受一个带非空 Observation ID 的 Fact；多个 direct candidates 返回 `ErrUnsupported`，不会静默合并 attributes。

Volcengine AgentKit/Viking MEM0 只通过一个 `volc.Config` 构造。它接收显式的 Mem0 data-plane key 或 credential resolver。Adapter 显式选择火山云 v1 add/search 路径，从 `results` 读取唯一权威 job ID，并让 `Wait` 轮询 `/v1/job/{id}/`。成功 job 不带 facts 时，Adapter 只列出同一 scope，并按 observation ID 选择该次写入的记录。火山云 v1 服务要求 `user_id`，因此 App-only、Agent-only 或 Run-only 逻辑 scope 会得到一个保留的完整 scope 编码 transport user，同时仍保留所有原始 native 字段；读取后会还原并按未改变的逻辑 scope 校验。普通 Mem0 Platform 仍使用 v3 add/search、顶层 event ID 和 `/v1/event/{id}/`。不能根据 endpoint hostname 推断协议。火山云 data-plane endpoint 必填。

Eino `memory_observe` node 会为每个 Graph 写入的 direct Fact 分配由当前 turn 与 Graph node 派生的稳定 observation identity。因此，同一 Workspace 的后续 `memory_recall` node 会使用相同完整 `Scope.AppID` 读到该 Fact，`volc_mem0` 绑定也遵守这一保证。火山云 search result 必须带 native Fact ID，并与编码后的 scope 兼容；project、strategy 等 provider routing metadata 不会作为业务 attributes 返回。

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

`MemoryLayout` 的三个 provider block 都必须存在。Flowcraft block 中的 extraction、embedding 和 rerank model 是 RuntimeProfile model alias，使用与 RuntimeProfile binding 相同的总长 1–63 字节、由 `.` 分隔的 lowercase kebab-case segment 语法。每个完整 alias 都是平面 map 中的 opaque key，只做精确解析，不支持 prefix、segment 或 fallback lookup；只有实际选择 `driver: flowcraft` 时才解析这些 alias。`extraction.enabled` 默认为 `true`；设为 `false` 时不运行模型提取，但 Graph 写入的 direct Facts 仍然可用。

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

Server 为每个 binding 只打开一次该物理 backend，并向每个 Workspace generation 提供独立关闭的 logical Store。当已发布的 Flowcraft projection signature 未改变时，不同 Workspace 的 logical Store 会并发构造，该过程不属于 binding registry map 的临界区。Policy 变化仍只有一个串行的 projection rebuild owner；完整 replacement 原子发布后，其他 constructor 才继续。正常的 final-lease cleanup 会在最后一个 logical lease 与在途 constructor 都退出后关闭物理 backend；显式 Registry shutdown 会先摘除 binding、拒绝晚到的 constructor 结果并排空这些 constructor，再关闭物理 backend。

合法的 Flowcraft connection 是 `flowcraft_postgresql`（`dsn`）和 `flowcraft_redis8`（`url`，可选 `tls_ca_file`）。`flowcraft_redis8` 要求 Redis 8.4 或更高版本及 Redis Search，Canonical Fact、Evidence、Async Semantic Queue、Side-effect Outbox 和全文/向量 retrieval 全部使用同一个 Redis namespace；它不降级支持 Redis 7 或 Redis 8.0/8.2。Retrieval 在 Redis 内执行 BM25、HNSW KNN、结构化 metadata filter、top-K 限制和 `FT.HYBRID` RRF 融合。`rediss://` 连接复用 Storage 的 TLS 校验，并可通过 `tls_ca_file` 增加受信 CA。Flowcraft 0.1.7 尚未公开 Graph store 注入点，因此该 connection 会拒绝 `graph_enabled`，避免静默使用不持久化的进程内 Graph。Driver 与 connection type 必须匹配，未知字段、缺失 key 和无效 endpoint 会在 RuntimeProfile 写入或解析时被拒绝。

Flowcraft 0.1.7 将 `(runtime_id, user_id)` 定义为 canonical hard partition。`agent_id` 是 soft-isolation metadata，因此会被有意排除在 `ScopeEnumerator` 之外；使用枚举出的 hard scope 仍可读回该分区内所有 AgentID 写入的 Fact，避免破坏 cross-agent recall。

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

同一 Workspace 的所有 stream 共用一个 Agent generation。数据可见性的稳定边界是同一 Workspace AppID、同一 memory driver 和同一 RuntimeProfile memory binding 指向的物理 connection。修改 extraction、recall、write、prompt、`top_k` 或 mode 不改变 canonical data；Flowcraft 派生索引 policy 改变时，从 canonical facts 在 staging index 中重建，成功后原子发布，失败不会发布部分或混合索引。切换 driver 或 binding 可以切换物理数据源，不自动迁移或删除；切回仍存在的原 connection 后可以重新访问原数据。

## Ownership 与错误

Provider adapter 不关闭注入的 dependency。构造 workspace、index、HTTP client 或 credential dependency 的 composition root 拥有它们，并按构造顺序的逆序关闭资源。

稳定的 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 保留 `errors.Is` 语义。无法完整保持 filter、attribute patch 或 conditional-write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃条件。错误不得暴露 API key、access-key credential 或带 credential 的 response body。

物理 Memory Store 构造按完整 binding key 协调：相同 key 的调用者共享一个 backend，无关 binding 可以独立构造。Direct Mem0 Fact 幂等边界是完整 canonical Scope 加 observation ID，因此慢 provider 请求不会停止无关 scope/observation；相同 key 的 retry 仍会 reconcile 或返回 `ErrConflict`。
