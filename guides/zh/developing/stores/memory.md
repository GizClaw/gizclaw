# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) 是 Agent runtime 共用的 provider-neutral 长期记忆边界。Flowcraft、Mem0 和 Volc 适配器分别位于 `flowcraft`、`mem0`、`volc` 子包。

## 契约

`Observation.Scope` 和 `Query.Scope` 是产品层拥有的不透明 namespace。上层决定哪些产品上下文共享一份记忆，但不需要知道或传入 runtime、user、agent、run、project、worker ID 等 provider-native 字段。每个适配器在包内完成协议映射。

```go
result, err := store.Observe(ctx, memory.Observation{
	Scope: memory.Scope("player:42"),
	Turns: turns,
})

recalled, err := store.Recall(ctx, memory.Query{
	Scope: memory.Scope("player:42"),
	Text:  "玩家偏好什么？",
	Limit: 10,
})
```

`Update` 和 `Delete` 只接收 store 返回的不透明 fact ID，以及可选的不透明 revision。`Wait` 只接收不透明 operation ID。基于 ID 的操作不再要求上层重复传 scope；适配器把再次路由所需的信息编码在 locator 内。

异步 `Observe` 返回 operation。实现 `OperationWaiter` 的 store 使用调用方已有的 `context.Context` 等待，不在 constructor 中启动后台 goroutine。使用相同持久化依赖重新构造 Flowcraft adapter 后，仍可恢复 durable operation locator。

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

Mem0 只通过一个 `mem0.Config` 构造。`FlavorPlatform` 使用 `Authorization: Token`；`FlavorSelfHosted` 在配置 key 时使用 `X-API-Key`。Adapter 在内部把 operation scope 映射为 Mem0 的 `user_id`，update/delete 直接使用返回的 memory ID。

Volcengine AgentKit/Viking MEM0 只通过一个 `volc.Config` 构造。它接收显式的 Mem0 data-plane key 或 credential resolver，解析 credential 后复用 Mem0 adapter；data-plane endpoint 必填。

## 组合与 YAML

`cmd/internal/stores` 是 composition root，负责 serializable YAML DTO、环境变量展开、workspace/index 构造、model loader 注入、credential 解析和 lifecycle。Flowcraft `dir` 属于这一层：command 创建 Flowcraft workspace 和 BBH retrieval index，再把对应 interface 注入 adapter。

```yaml
stores:
  agent-memory:
    kind: memory
    flowcraft:
      dir: ${GIZCLAW_MEMORY_DIR}
      extraction_model: memory-extractor
      embedding_model: text-embedding
      extraction_mode: single_pass
      graph_enabled: true
      async:
        enabled: true
```

Mem0 Platform：

```yaml
stores:
  agent-memory:
    kind: memory
    mem0:
      endpoint: https://api.mem0.ai
      api_key: ${MEM0_API_KEY}
      flavor: platform
```

Volcengine AgentKit/Viking MEM0：

```yaml
stores:
  agent-memory:
    kind: memory
    volc_memory:
      mem0:
        endpoint: ${VOLC_MEM0_ENDPOINT}
      memory_project_id: ${VOLC_MEMORY_PROJECT_ID}
      region: cn-beijing
      access_key_id: ${VOLC_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_ACCESS_KEY_SECRET}
```

一个 logical memory store 必须只选择一个 provider。未知 YAML 字段会被拒绝；scope 和 backend-native routing 字段都不是合法的 server 配置。

## Ownership 与错误

Provider adapter 不关闭注入的 dependency。构造 workspace、index、HTTP client 或 credential dependency 的 composition root 拥有它们，并按构造顺序的逆序关闭资源。

稳定的 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 保留 `errors.Is` 语义。无法完整保持 filter、attribute patch 或 conditional-write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃条件。错误不得暴露 API key、access-key credential 或带 credential 的 response body。
