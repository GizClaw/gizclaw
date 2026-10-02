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

`ScopePurger` 是不可逆删除一个 scope 的可选 capability。Purge 集合精确等于使用同一 `Scope` 的 `Recall` 能返回的 Fact，再加上之后可能 materialize 进这个集合的 provider-owned 记录（待处理 extraction job、派生索引与 provenance marker）。`PurgeScope` 幂等；native bulk delete 无法精确表达该集合时返回 `ErrUnsupported`，不会多删或少删。`ScopeEmpty` 报告 purge 集合是否仍有 Fact。调用方通过 `memory.PurgeScope` 与 `memory.ScopeEmpty` 使用该能力，它们会穿过 `BindApp` view 并先应用 view 的 AppID 绑定；底层没有该能力时返回 `ErrUnsupported`。

| Provider | Purge | 校验 |
| --- | --- | --- |
| Flowcraft | `ForgetAll(ForgetHard)` 删除 `(runtime_id, user_id)` hard partition 的 canonical fact、marker、所有 projection、evidence，以及该 partition 的 async semantic 与 side-effect job。`AgentID` 是 partition 内的 soft metadata，选择 `AgentID` 的 scope 返回 `ErrUnsupported` | canonical temporal store 中该 partition 不再有任何 revision |
| Mem0 Platform | `DELETE /v1/memories/` 按已选择维度 AND 过滤；值为 `*` 的维度是 provider wildcard，返回 `ErrInvalidInput` | `POST /v3/memories/` 列出同一 filter |
| Mem0 self-hosted | `DELETE /memories?user_id=<完整 scope 编码>` | `GET /memories` 使用同一 `user_id` |
| Volc | `DELETE /v1/memories/?user_id=<保留 scope user>`。Volc bulk delete 只接受 `user_id`、`agent_id`、`run_id`，无法把调用方选择的 `UserID` 限定到一个 App，这类 scope 返回 `ErrUnsupported` | `GET /v1/memories/` 使用写入时的维度 |

Mem0 Platform 异步删除；Volc 的 `async_mode` add job 可能在 purge 之后才 materialize。需要持久保证的调用方反复 purge 并校验，直到 `ScopeEmpty` 返回 true。Flowcraft 的 `NewMaintenance` 在调用方拥有的持久依赖上构造只用于 purge 与校验的 Store：它不加载 model，允许没有 extraction model 的 async queue，以便 purge 同时取消排队 job；其 `Observe` 与 `ProcessAsync` 返回 `ErrUnsupported`。

## Provider 构造

Provider 包只接收内存中的 runtime dependency，不解析 YAML、不展开环境变量、不读取配置文件，也不决定产品身份。

Flowcraft 只通过一个 `flowcraft.Config` 构造。该结构可注入 `ModelLoader`、retrieval index、temporal store、evidence store、async queue 和 side-effect outbox。注入的 dependency 仍由调用方拥有；没有注入时，adapter 使用 Flowcraft 的内存实现。side-effect outbox job 到达注入的 outbox 之前，adapter 只在 job 没有 ID 且 request ID 非空时为其分配 scope 限定的 identity（`<scope canonical key>|<request ID>|<kind>`）；调用方自带的 ID 和没有 request ID 的 job 原样透传。Flowcraft 自身的 Save batch 总是不带 ID 到达，因此共享同一个 outbox 的不同 scope 并发 Save 不会互相去重 projection、embedding 或 evolution job，而同一 scope 的 batch 重放仍保持幂等。

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

Volcengine AgentKit/Viking MEM0 只通过一个 `volc.Config` 构造。它接收显式的 Mem0 data-plane key 或 credential resolver。Adapter 显式选择火山云 v1 add/search 路径，从 `results` 读取唯一权威 job ID，并让 `Wait` 轮询 `/v1/job/{id}/`。成功 job 不带 facts 时，Adapter 只列出同一 scope，并按该次 operation 的 reconciliation marker 选择记录。成功提取可返回零条 facts，例如纯问候；已有记忆不会作为本次结果返回，缺失或无效的列表响应仍会报错。火山云 v1 服务要求 `user_id`，因此 App-only、Agent-only 或 Run-only 逻辑 scope 会得到一个保留的完整 scope 编码 transport user，同时仍保留所有原始 native 字段；读取后会还原并按未改变的逻辑 scope 校验。普通 Mem0 Platform 仍使用 v3 add/search、顶层 event ID 和 `/v1/event/{id}/`。不能根据 endpoint hostname 推断协议。火山云 data-plane endpoint 必填。

Eino `memory_observe` node 会为每个 Graph 写入的 direct Fact 分配由当前 turn 与 Graph node 派生的稳定 observation identity。因此，同一 Workspace 的后续 `memory_recall` node 会使用相同完整 `Scope.AppID` 读到该 Fact，`volc_mem0` 绑定也遵守这一保证。火山云 search result 必须带 native Fact ID，并与编码后的 scope 兼容；project、strategy 等 provider routing metadata 不会作为业务 attributes 返回。

### 自托管 Mem0 服务

`cmd/mem0/gizclaw_mem0` 是独立 Python HTTP 入口，使用官方 `mem0ai 2.2.1` SDK。
服务支持 OpenAI-compatible LLM/Embedding，向量存储为 PGVector 或本地 Qdrant；
`build/mem0/Dockerfile` 提供 `test` 与非 root `runtime` target。Docker E2E、LoCoMo 和
Release 使用同一份服务实现。提取模型和 Embedding 属于该服务配置，RuntimeProfile
只保存 endpoint 与可选 API key。

配置文件通过 `--config` 或 `MEM0_CONFIG` 选择。`memory` 对象使用 Mem0 原生模型/存储配置，拒绝全局业务 `custom_instructions`；
`service` 支持 `api_key`、`thinking`、`embedding_protocol` 和 `max_concurrency`。模型档位配置为 `memory.llm.config.service_tier`；wrapper 在构造原生 Mem0 config 前取出该扩展字段，再通过 OpenAI SDK 转发。`${VARIABLE}` 从进程环境
展开，缺失或空值会阻止启动；配置文件模式下，模型/数据库配置以文件为准，不与对应的
`MEM0_*` 环境配置混合。不传文件时保留环境配置入口。

- `cmd/mem0/config.example.yaml` 使用 Seed 2.1 Lite、豆包 Vision Embedding、PGVector。
  `service.thinking: disabled` 通过 OpenAI-compatible Chat API 发送 Ark 参数。
- 国内示例设置 `memory.llm.config.service_tier: fast`，通过 Chat API 请求火山低延迟档位；环境模式使用
  `MEM0_LLM_SERVICE_TIER=fast`。服务健康检查回显请求档位，提取诊断记录响应的实际
  `service_tier`。需要账户开通低延迟服务，额度不足或触发流量保护可能回退 default；
  fast 不绕过模型账户 TPM 限额。
- OpenAI 也支持 Fast mode；支持的模型可设置 `service_tier: fast` 或 `priority`，具体账号和模型支持以[官方文档](https://developers.openai.com/api/docs/guides/fast-mode#configuring-fast-mode)为准。OpenAI 示例使用 `auto`。
- `cmd/mem0/config.openai.example.yaml` 使用 `gpt-6-luna`、`text-embedding-3-small`、
  PGVector。Luna 的 `max_tokens` 被映射为 `max_completion_tokens`；使用
  `reasoning_effort: none`，其余 reasoning 模式省略不兼容的采样参数。请求格式已有
  本地测试，账号可用性和真实模型质量须通过 live 测试验收。
- `embedding_protocol: openai` 使用标准 `/embeddings`；`ark_multimodal` 使用
  `/embeddings/multimodal`，为每条文本单独生成 1024 或 2048 维向量，并区分 Corpus/
  Query instruction。切换 Embedding 模型或维度须使用新 collection 并重新入库。

在仓库根目录启动 Python 服务；先注入示例所需的模型 key、`MEM0_POSTGRES_DSN` 和
`MEM0_API_KEY`，目标 PostgreSQL 必须支持 `vector` extension：

```sh
python3 -m venv .tmp/mem0-venv
.tmp/mem0-venv/bin/pip install -r cmd/mem0/requirements.txt
mkdir -p .tmp/mem0
export MEM0_HISTORY_DB_PATH="$PWD/.tmp/mem0/memory-history.db"
PYTHONPATH=cmd/mem0 .tmp/mem0-venv/bin/python -m gizclaw_mem0 \
  --config cmd/mem0/config.example.yaml --host 127.0.0.1 --port 8000
```

容器示例在仓库根目录执行，沿用已注入的凭据；连接托管 PG 时数据库数据与向量索引
保存在 PG，本地 `/data` 保存 SDK 的 SQLite history。该 volume 应持久化：

```sh
PLATFORM=linux/amd64 IMAGE=gizclaw-mem0 build/build-mem0.sh
# 国内依赖源：同一应用 Dockerfile，选择 CN base
PLATFORM=linux/amd64 IMAGE=gizclaw-mem0 build/build-mem0.sh cn
docker run --rm --name gizclaw-mem0 -p 127.0.0.1:8000:8000 \
  -e VOLC_ARK_API_KEY -e MEM0_POSTGRES_DSN -e MEM0_API_KEY \
  -v "$PWD/cmd/mem0/config.example.yaml:/app/config.yaml:ro" \
  -v gizclaw-mem0-history:/data gizclaw-mem0 --config /app/config.yaml --host 0.0.0.0
```

Python 默认监听 loopback；容器内监听 `0.0.0.0`，示例仅映射宿主机 loopback。
配置 API key 后，除 `/health` 外的请求都须携带 `X-API-Key`。省略 key 的本机模式应
限定在可信网络。停止服务会关闭模型客户端、向量库和 history 连接；服务不提供在线
修改模型/数据库配置的接口。PG 后端允许独立实体的操作并发；本服务进程内，共用任一原生实体的写入与删除串行执行，覆盖复合实体写入和较宽范围的清理。LLM 提取结果在 SDK 持久化前校验，不完整或无效结果不能先落库再返回提取失败。提取诊断与
Embedding 错误状态按请求线程隔离。Qdrant 本地状态仍串行访问。
`max_concurrency` 是写入额度，默认为 160；读取另预留 `max(16, max_concurrency/4)`
额度。超额请求返回 503，健康检查不占该额度。模型供应商的限流返回结构化 HTTP 429，
其他模型请求失败返回 502；错误响应不透传供应商凭据。PG pool 默认 min 4 / max 32，环境模式可通过
`MEM0_MAX_CONCURRENCY`、`MEM0_POSTGRES_MIN_CONNECTIONS`、
`MEM0_POSTGRES_MAX_CONNECTIONS` 配置。LoCoMo 验证质量，吞吐量由独立 load test 验证。

HTTP contract 由 `api/http/mem0.json` 定义，运行时 `/openapi.json` 返回该契约；
Python contract test 校验实际请求/响应模型、参数、路由和 operation ID。
`sdk/go/mem0` 由根 module 固定版本的 `oapi-codegen` 生成：

```sh
go generate ./sdk/go/mem0
go test ./sdk/go/mem0 ./pkgs/store/memory/mem0
```

GizClaw 的 self-hosted adapter 使用生成的 client 和请求 DTO；Platform/Volc
仍使用各自协议。SDK 保留 Mem0 record 的额外 metadata。

多个 MemoryLayout 可共用同一服务。`MemoryLayout.mem0.custom_instructions` 由 Go logical
Store 保留为该 Layout generation 的独立 policy，Observe 时通过 HTTP `prompt` 传给
原生 `Memory.add(prompt=...)`；不修改共享 SDK 的全局 instruction。更新 Layout
不会改写已有 generation 的 policy。`infer=false` direct Fact 不携带提取 instruction。
服务无需另外注册 Layout；`scope: workspace|peer` 仍按现有 Scope 契约执行。Layout
本身不增加数据分区；同 endpoint/collection 和完整 Scope 共享记忆。OSS binding 不支持
`custom_categories`、启用 `decay` 或启用 `multilingual` flag，配置时会拒绝。

同版本 amd64/arm64 镜像的发布规则见[仓库发布](../tooling#仓库发布)。

## MemoryLayout、RuntimeProfile 与 Workflow

Memory 不再是 Server Config 中的 `stores.kind: memory`。Portable policy、部署连接和 Graph 消费行为分属三个资源面：

- Admin `MemoryLayout` 同时声明 Flowcraft、Mem0 和 `volc_mem0` 的 provider policy，不包含 endpoint、API key、DSN 或目录。每种实现独立配置 `scope: workspace|peer`；省略时使用现有的 Workspace 隔离行为。
- RuntimeProfile 的 `resources.memories.<alias>` 选择 Layout、实际 driver 和严格类型化 connection。Connection 中的 endpoint、API key、project ID、DSN 或目录直接属于该 RuntimeProfile，不引用 Credential 资源。
- Workflow 顶层 `memory` 只引用 RuntimeProfile alias。Graph 的 `memory_recall` / `memory_observe` node 决定何时读写、query 从哪里来、结果写到哪里，以及如何从 turn 或 state 构造 fact；这些映射不属于 MemoryLayout。

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata:
  name: pet-memory
spec:
  flowcraft:
    scope: peer
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
    scope: peer
    custom_instructions: Extract durable pet and owner facts.
  volc_mem0:
    scope: peer
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

Server 为每个 binding 只打开一次该物理 backend，并向每个 Workspace generation 提供独立关闭的 logical Store。当已发布的 Flowcraft projection signature 未改变时，不同 Workspace 的 logical Store 会并发构造，该过程不属于 binding registry map 的临界区。Policy 变化仍只有一个串行的 projection rebuild owner；完整 replacement 原子发布后，其他 constructor 才继续。Resolve 在离开 registry 锁之前保留 binding。正常的 final-lease cleanup 会在最后一个 logical lease 与在途 Resolve 都退出后关闭物理 backend；显式 Registry shutdown 会先摘除 binding、拒绝晚到的 constructor 结果并排空这些 constructor，再关闭物理 backend。

合法的 Flowcraft connection 是托管本地 `flowcraft_bbh`、`flowcraft_object_store`（`directory`）、`flowcraft_postgresql`（`dsn`）和 `flowcraft_redis8`（`url`，可选 `tls_ca_file`）。`flowcraft_bbh` 不依赖外部服务，每个 binding 的数据位于 `<server-root>/data/memory/<profile-id-hash>/<binding>`；可选的 `flowcraft.bbh` Layout policy 控制 BBH search overfetch、Bleve analyzer 和 HNSW flush。`flowcraft_redis8` 要求 Redis 8.4 或更高版本及 Redis Search，Canonical Fact、Evidence、Async Semantic Queue、Side-effect Outbox 和全文/向量 retrieval 全部使用同一个 Redis namespace；它不降级支持 Redis 7 或 Redis 8.0/8.2。Retrieval 在 Redis 内执行 BM25、HNSW KNN、结构化 metadata filter、top-K 限制和 `FT.HYBRID` RRF 融合。`rediss://` 连接复用 Storage 的 TLS 校验，并可通过 `tls_ca_file` 增加受信 CA。Flowcraft 0.1.7 尚未公开 Graph store 注入点，因此 Redis8 connection 会拒绝 `graph_enabled`，避免静默使用不持久化的进程内 Graph。Driver 与 connection type 必须匹配，未知字段、缺失 key 和无效 endpoint 会在 RuntimeProfile 写入或解析时被拒绝。

Flowcraft 0.1.7 将 `(runtime_id, user_id)` 定义为 canonical hard partition。`agent_id` 是 soft-isolation metadata，因此会被有意排除在 `ScopeEnumerator` 之外；使用枚举出的 hard scope 仍可读回该分区内所有 AgentID 写入的 Fact，避免破坏 cross-agent recall。

已删除的 `flowcraft_bbh` connection 不提供自动迁移。使用该 connection 的已持久化 legacy profile 会 fail closed 并返回可操作的替换错误，但 mutation path 仍允许管理员把 profile 替换为受支持的 connection。旧 managed directory 及其中的 canonical data 保持原样；替换或删除 profile 都不会删除该目录。`flowcraft_object_store` 可以继续在内部使用其本地 derived index，但 BBH 不再是公开的部署 connection 或 policy surface。

对于 Mem0 和火山云，Project ID 记录与所选数据面 API key 配套的部署/控制面身份。运行时 Fact 请求通过该 key 完成 Project 路由，不会再发送独立的 Project ID 字段。

自托管 Mem0 使用 `driver: mem0` 和 `connection.type: mem0_self_hosted`，显式选择 OSS HTTP 协议。只要求 `endpoint`；服务启用认证时可提供 `api_key`，Adapter 通过 `X-API-Key` 发送。未启用认证的本机服务省略该字段。Self-hosted connection 不接受 `project_id`、数据库 DSN 或模型配置；向量库、模型和持久化由 Mem0 服务管理。原有 `connection.type: mem0` 仍选择 Platform 协议，并要求 `project_id` 与 `api_key`；不会根据 endpoint 自动推断协议。

```yaml
spec:
  resources:
    memories:
      assistant-memory:
        layout_id: assistant-memory
        driver: mem0
        connection:
          type: mem0_self_hosted
          endpoint: http://127.0.0.1:18000
```

自托管提取会把 `Turn.Speaker` 与 UTC `ObservedAt` 放入消息正文，因为 OSS parser 不消费 OpenAI `name` 字段。turn 未提供时间时使用 Observation 时间。Platform/Volc 消息正文及 `infer=false` direct Fact 不做此变换。

该绑定使用 MemoryLayout 的 `mem0.scope` 选择 Workspace 或 Peer 记忆归属，并复用 OSS Adapter 的完整 Scope 编码、读写、修改、删除与 purge 校验。

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

同一 Workspace 的所有 stream 共用一个 Agent generation。MemoryLayout 中当前 driver 的 `scope` 决定长期记忆归属：`workspace` 将 Workspace ID 映射到公共 `Scope.AppID`；`peer` 将 Workspace owner Peer public key 派生为保留的 Peer AppID。Flowcraft 将 AppID 映射到 RuntimeID，Mem0 Platform 与火山云映射到 app_id；自托管 Mem0 将完整逻辑 Scope 编码到原生 user_id。读、写、统计与清理使用相同映射，Workflow Graph 和记忆节点不感知归属。不同 Workspace 要共享同一 Peer 记忆，还必须使用同一个物理数据空间：Flowcraft Peer scope 的同一 RuntimeProfile、同一 Layout 与同一连接会跨 binding alias 复用物理空间；托管本地目录放在独立的 `data/peer-memory` 根目录，Redis 8 使用独立的 `peer:` namespace，避免与同名 Workspace alias 冲突。Workspace scope 仍按 alias 隔离。Mem0/火山云须路由到同一 provider project；相同 Peer AppID 不会跨不同物理连接自动合并数据。切换 driver、binding 或 scope 不迁移旧数据。

删除 Workspace 时仅清除 `scope: workspace` 的长期记忆；`scope: peer` 的共享记忆在该 Peer 删除时清理并校验。Workspace deletion handler 在 quiesce runtime 后，通过 retained Workspace row、Workflow `memory` alias 与 owner 当前 RuntimeProfile 解析 binding，对于 `scope: workspace` 调用 `Registry.PurgeWorkspace` 删除 `Scope{AppID: <Workspace ID>}`，只有 `Registry.WorkspaceMemoryEmpty` 确认为空后才 finalize；`scope: peer` 的 Workspace 删除不触碰共享记忆；仍有残留时返回 retryable `memory_residual` 并在下次重试时再次 purge。该 purge 以 maintenance 模式打开与 runtime 共享的物理 backend：不加载 model，也不重建派生索引，因此 owner 的 model catalog 不可用时仍可 purge；本地派生索引 policy 过期时保留已发布的 manifest，由下一个 runtime Store 负责重建。Workspace、Workflow、owner RuntimeProfile、memory alias 或 MemoryLayout 已不存在时，没有可解析的当前 binding，handler 视为无需清除；resolver 或 provider 的临时失败保持 retryable；provider 无法表达的 scope 以 terminal `memory_cleanup_unsupported` 停止，交由运维处理。GizClaw 不记录 binding 历史，Workspace 在更早的 driver、connection 或 alias 下写入的数据不会被这次 purge 访问。

## Ownership 与错误

Provider adapter 不关闭注入的 dependency。构造 workspace、index、HTTP client 或 credential dependency 的 composition root 拥有它们，并按构造顺序的逆序关闭资源。

稳定的 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 保留 `errors.Is` 语义。无法完整保持 filter、attribute patch 或 conditional-write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃条件。错误不得暴露 API key、access-key credential 或带 credential 的 response body。

物理 Memory Store 构造按完整 binding key 协调：相同 key 的调用者共享一个 backend，无关 binding 可以独立构造。Direct Mem0 Fact 幂等边界是完整 canonical Scope 加 observation ID，因此慢 provider 请求不会停止无关 scope/observation；相同 key 的 retry 仍会 reconcile 或返回 `ErrConflict`。
