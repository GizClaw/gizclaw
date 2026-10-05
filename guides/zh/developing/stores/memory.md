# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) 是 Agent runtime 共用的 provider-neutral 长期记忆边界。Mem0 Cloud、自托管 Mem0 和 Volc 适配器位于 `mem0` 与 `volc` 子包。

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

| 公共字段 | Mem0 / Volc Memory |
| --- | --- |
| `AppID` | `app_id` |
| `UserID` | `user_id` |
| `AgentID` | `agent_id` |
| `RunID` | `run_id` |

Mem0/Volc 支持这些独立维度及其组合，发送全部已选择维度，并在 recall 与变更校验中使用 AND filter。自托管 Mem0 的完整 Scope 编码见下文。

`Text` 和 `Turns` 是待提取的原始材料；`Facts` 是上层已经结构化的候选事实。Provider 必须保持候选事实的文本与其支持的 attributes，无法直接写入时返回 `ErrUnsupported`，不能把候选事实静默送回模型二次提取。Mem0/Volc 使用 `infer=false` direct import，保留候选文本和 attributes。

对于包含 direct Facts 的 Observation，非空 `Observation.ID` 是完整 `Scope` 内的幂等键。相同 ID 和相同 canonical direct-Fact payload 的并发调用或重试返回原 logical Fact 或 durable operation；Fact 文本、attributes 或 `ObservedAt` 改变时返回 `ErrConflict`。Adapter 在 native record 中保存 payload digest；self-hosted 的 durable reservation 和精确逐项对账由服务拥有，其他 Mem0 adapter 在提交前先对账；因此 provider 已接受但 response 丢失后的重试不会创建第二个 logical Fact。返回的 `Fact.Sources` 保留 `ObservationID`。这些 provider-owned metadata 不会暴露为业务 attributes。模型 extraction 的 provider-native dedup 行为不属于这个 direct-Fact 保证。

`UpdateRequest`、`DeleteRequest` 和 `OperationRequest` 都必须重新携带调用方的 `Scope`，以及 Store 返回的不透明 fact、revision 或 operation locator。Locator 不是授权来源：Adapter 在 mutation 或完成异步操作前校验请求 Scope 与 locator、provider record 一致。原始 provider ID 不能绕过 App 边界。

异步 `Observe` 返回 operation。实现 `OperationWaiter` 的 Store 使用调用方的 context 等待，并在恢复 operation 时校验 locator 与完整 Scope。Constructor 不启动后台 worker。

`memory.BindApp(store, appID)` 返回一个借用的 Store view。它只填充或校验 `Scope.AppID`，不生成、清空、拼接、hash 或改写调用方的 `UserID`、`AgentID` 和 `RunID`。冲突 AppID 返回 `ErrInvalidInput`。View 不拥有也不关闭底层 Store，并且只有在底层实现 `OperationWaiter`、`AsyncOperationProcessor` 或 `StatisticsProvider` 时才暴露相同 capability。

`ScopePurger` 是不可逆删除一个 scope 的可选 capability。Purge 集合精确等于使用同一 `Scope` 的 `Recall` 能返回的 Fact，再加上之后可能 materialize 进这个集合的 provider-owned 记录（待处理 extraction job、派生索引与 provenance marker）。`PurgeScope` 幂等；native bulk delete 无法精确表达该集合时返回 `ErrUnsupported`，不会多删或少删。`ScopeEmpty` 报告 purge 集合是否仍有 Fact。调用方通过 `memory.PurgeScope` 与 `memory.ScopeEmpty` 使用该能力，它们会穿过 `BindApp` view 并先应用 view 的 AppID 绑定；底层没有该能力时返回 `ErrUnsupported`。

| Provider | Purge | 校验 |
| --- | --- | --- |
| Mem0 Platform | `DELETE /v1/memories/` 按已选择维度 AND 过滤；值为 `*` 的维度是 provider wildcard，返回 `ErrInvalidInput` | `POST /v3/memories/` 列出同一 filter |
| Mem0 self-hosted | `DELETE /memories?user_id=<完整 scope 编码>` | `GET /memories` 使用同一 `user_id` |
| Volc | `DELETE /v1/memories/?user_id=<保留 scope user>`。Volc bulk delete 只接受 `user_id`、`agent_id`、`run_id`，无法把调用方选择的 `UserID` 限定到一个 App，这类 scope 返回 `ErrUnsupported` | `GET /v1/memories/` 使用写入时的维度 |

Mem0 Platform 异步删除；Volc 的 `async_mode` add job 可能在 purge 之后才 materialize。需要持久保证的调用方反复 purge 并校验，直到 `ScopeEmpty` 返回 true。
## Provider 构造

Provider 包只接收内存中的 runtime dependency，不解析 YAML、不展开环境变量、不读取配置文件，也不决定产品身份。

Mem0 只通过一个 `mem0.Config` 构造。`FlavorPlatform` 使用 `Authorization: Token`，并将所有已选择的维度映射到对应的 `app_id`、`user_id`、`agent_id` 和 `run_id`。Mem0 OSS 不提供 `app_id`，因此 `FlavorSelfHosted` 会把完整四维 Scope 编码到一个保留的原生 `user_id` 中；配置 key 时使用 `X-API-Key`。这样既能精确保持 Workspace App 隔离，也不会改写调用方逻辑上的 User、Agent 或 Run 维度。Update/Delete 先读取 provider record 并校验完整编码 scope，再执行 ID mutation。Self-hosted direct import 一次接受 1–1000 个带非空 Observation ID 的 Facts，通过一个 HTTP 请求提交；Platform 与 Volc 仍一次接受一个 Fact，多个 candidates 返回 `ErrUnsupported`。每条 Fact 的原始文本、attributes 和完整 Scope 独立保留。

Volcengine AgentKit/Viking MEM0 只通过一个 `volc.Config` 构造。它接收显式的 Mem0 data-plane key 或 credential resolver。Adapter 显式选择火山云 v1 add/search 路径，从 `results` 读取唯一权威 job ID，并让 `Wait` 轮询 `/v1/job/{id}/`。成功 job 不带 facts 时，Adapter 只列出同一 scope，并按该次 operation 的 reconciliation marker 选择记录。成功提取可返回零条 facts，例如纯问候；已有记忆不会作为本次结果返回，缺失或无效的列表响应仍会报错。火山云 v1 服务要求 `user_id`，因此 App-only、Agent-only 或 Run-only 逻辑 scope 会得到一个保留的完整 scope 编码 transport user，同时仍保留所有原始 native 字段；读取后会还原并按未改变的逻辑 scope 校验。普通 Mem0 Platform 仍使用 v3 add/search、顶层 event ID 和 `/v1/event/{id}/`。不能根据 endpoint hostname 推断协议。火山云 data-plane endpoint 必填。

Eino `memory_observe` node 会为每个 Graph 写入的 direct Fact 分配由当前 turn 与 Graph node 派生的稳定 observation identity。因此，同一 Workspace 的后续 `memory_recall` node 会使用相同完整 `Scope.AppID` 读到该 Fact，`volc_mem0` 绑定也遵守这一保证。火山云 search result 必须带 native Fact ID，并与编码后的 scope 兼容；project、strategy 等 provider routing metadata 不会作为业务 attributes 返回。

### 自托管 Mem0 服务

`cmd/mem0/gizclaw_mem0` 是独立 Python HTTP 入口，使用官方 `mem0ai 2.2.1` SDK。
服务支持 OpenAI-compatible LLM/Embedding，向量存储为 PGVector 或本地 Qdrant；
`build/mem0/Dockerfile` 提供 `test` 与非 root `runtime` target。Docker E2E、LoCoMo 和
Release 使用同一份服务实现。提取模型和 Embedding 属于该服务配置，RuntimeProfile
只保存 endpoint 与可选 API key。

PGVector 集合在服务启动期间按原生 SDK 初始化，完成后才接受请求，避免冷启动时并发 search/add 竞争建表。集合初始化失败使服务启动失败，并关闭已构造的客户端与连接池。

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
修改模型/数据库配置的接口。PG 后端允许独立实体的 Observe 并发；本服务进程内，共用任一原生实体的写入与删除串行执行，覆盖复合实体写入和较宽范围的清理。Purge 与按 ID 更新/删除还共用解析协调锁，在读取 ID 记录前获取，防止解析实体到提交之间被清理；这些维护操作彼此串行，不阻塞独立实体的 Observe。LLM 提取结果在 SDK 持久化前校验，不完整或无效结果不能先落库再返回提取失败。提取诊断与
Embedding 错误状态按请求线程隔离。Qdrant 本地状态仍串行访问。
`max_concurrency` 是写入额度，默认为 160；读取另预留 `max(16, max_concurrency/4)`
额度。超额请求返回 503，健康检查不占该额度。模型供应商的限流返回结构化 HTTP 429，
其他模型请求失败返回 502；错误响应不透传供应商凭据。PG pool 默认 min 4 / max 32，环境模式可通过
`MEM0_MAX_CONCURRENCY`、`MEM0_POSTGRES_MIN_CONNECTIONS`、
`MEM0_POSTGRES_MAX_CONNECTIONS` 配置。LoCoMo 验证质量，吞吐量由独立 load test 验证。

Self-hosted 的 `POST /memories` 在 `infer=false` 时支持成对的
`observation_id`、`observation_digest` 和每条 message 的 `metadata`。Go adapter
将整个 Observation 的 canonical digest（包含原始文本、attributes、顺序和
`ObservedAt`）传给服务。服务在首次写入前保存完整 wire payload 的摘要，变更
payload 返回 HTTP 409 / `ErrConflict`，同 payload 重试按原顺序返回全部 Facts。
未提供 identity 的旧 HTTP direct 调用和 `infer=true` 提取保持原行为；message
metadata 只用于带 identity 的 direct 调用。Provider-owned metadata 和原生
routing/history 字段不接受业务覆盖。

PostgreSQL 在 Mem0 数据库的 `gizclaw_direct_observations` 表中保存 reservation
和完成后的 Fact IDs；以 collection、完整 native routing 和 observation ID
为主键，并用 native entity 的 advisory locks 协调不同服务进程。Qdrant 保持
单进程 ownership，在 history 文件旁的 `.observations.db` 保存 reservation。
每个缺失项通过官方 `Memory.add(infer=false)` 使用独立 metadata 写入；通过精确
routing/observation/index vector listing 确认，不依赖 embedding 相似度或搜索排名。
只有全部候选唯一持久化才返回成功。写入后响应丢失、第二项失败或进程重启后，
相同 payload 重试只补缺失项，不重复已存在的记录。旧单条记录通过原 observation/digest 精确对账并保留原 Fact ID。完成后被显式删除的 Fact
不会被旧请求重建；重试返回 conflict。Scope purge 同时删除匹配的 reservation。
批次不具备跨请求原子事务：失败前的部分 Facts 可以被召回，调用方应重试完成
或 purge 清理。修改、删除和 purge 继续校验 Scope。

HTTP contract 由 `api/http/mem0.json` 定义，运行时 `/openapi.json` 返回该契约；
Python contract test 校验实际请求/响应模型、参数、路由和 operation ID。
`sdk/go/mem0` 由根 module 固定版本的 `oapi-codegen` 生成：

```sh
go generate ./sdk/go/mem0
go test ./sdk/go/mem0 ./pkgs/store/memory/mem0
```

GizClaw 的 self-hosted adapter 使用生成的 client 和请求 DTO；Platform/Volc
仍使用各自协议。SDK 保留 Mem0 record 的额外 metadata。

多个 MemoryLayout 可共用同一服务。`MemoryLayout.mem0_self_hosted.custom_instructions` 由 Go logical
Store 保留为该 Layout generation 的独立 policy，Observe 时通过 HTTP `prompt` 传给
原生 `Memory.add(prompt=...)`；不修改共享 SDK 的全局 instruction。更新 Layout
不会改写已有 generation 的 policy。`infer=false` direct Fact 不携带提取 instruction。
服务无需另外注册 Layout；`scope: workspace|peer` 仍按现有 Scope 契约执行。Layout
本身不增加数据分区；同 endpoint/collection 和完整 Scope 共享记忆。OSS binding 不支持
`custom_categories`、启用 `decay` 或启用 `multilingual` flag，配置时会拒绝。

同版本 amd64/arm64 镜像的发布规则见[仓库发布](../tooling#仓库发布)。

## MemoryLayout、RuntimeProfile 与 Workflow

Portable policy、部署连接和 Graph 消费行为分属三个资源面：

- Admin `MemoryLayout` 声明 Mem0 Cloud、自托管 Mem0 和 Volc 的 provider policy，不包含 endpoint、API key、数据库或模型连接。每种 policy 独立选择 `scope: workspace|peer`，省略时使用 Workspace scope。
- RuntimeProfile 的 `resources.memories.<alias>` 选择 Layout、driver 和唯一的 typed connection。Connection 由该 Admin 资源持有，不引用 Credential，也不会投影到 Peer API。
- Workflow 顶层 `memory` 引用 RuntimeProfile alias。Eino 的 `memory_recall` / `memory_observe` node 决定 query、filter、原始抽取材料与 direct Facts；这些映射属于 Graph。

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: MemoryLayout
metadata:
  id: pet-memory
spec:
  mem0:
    scope: peer
    custom_instructions: Extract durable pet and owner facts.
  mem0_self_hosted:
    scope: peer
    custom_instructions: Extract durable pet and owner facts.
  volc_mem0:
    scope: peer
    strategies:
    - name: owner-profile
      type: user_preference
      custom_instructions: Extract durable pet and owner facts.
```

`mem0` 与 `volc_mem0` block 必须存在。`mem0_self_hosted` 是独立的可选 block；选择对应 connection 时必须显式声明它，允许 `{}`。它只支持 `scope` 与 `custom_instructions`，不继承 Cloud 的 categories、multilingual 或 decay。构造、reload、读写、统计和清理始终按 connection type 选择同一个 policy。

支持的 connection 为 `mem0`（Cloud endpoint、API key、Project ID）、`mem0_self_hosted`（endpoint、可选 API key）和 `volc_mem0`（endpoint、API key、Memory Project ID）。`mem0` driver 接受前两种，`volc_mem0` driver 只接受同名 connection。无效字段、缺失参数及 driver/connection 不匹配会在写入或解析时被拒绝。

自托管 Mem0 显式选择 OSS 协议；启用认证时通过 `X-API-Key` 发送 key。数据库、提取模型、Embedding 和持久化由 Mem0 服务管理。Cloud/Volc 的 Project ID 是与 API key 配套的控制面身份，Fact 请求由 key 路由，不附加独立的 Project ID 参数。协议不会根据 endpoint hostname 推断。

```yaml
spec:
  resources:
    memories:
      pet-memory:
        layout_id: pet-memory
        driver: mem0
        connection:
          type: mem0_self_hosted
          endpoint: http://127.0.0.1:18000
```

自托管 extraction 将 `Turn.Speaker` 和 UTC `ObservedAt` 写进消息正文，turn 未提供时间时使用 Observation 时间；Cloud/Volc 和 direct Facts 保留原始文本。

下面的 Eino Graph 先召回，再生成回复，并把本轮 assistant 加入原始 conversation 材料后提交 extraction：

```yaml
spec:
  driver: eino
  memory: pet-memory
  eino:
    graph:
      name: companion
      compile:
        node_trigger_mode: any_predecessor
      state:
        fields:
        - name: memory_context
          type: string
          merge: replace
        - name: messages
          type: messages
          merge: replace
        - name: answer
          type: string
          merge: replace
        - name: turns
          type: messages
          merge: replace
      nodes:
      - id: recall
        type: memory_recall
        query_from: input.text
        output: memory_context
        top_k: 5
      - id: prompt
        type: prompt
        format: f_string
        inputs:
          memory:
            from: memory_context
          text:
            from: input.text
        outputs:
          messages: messages
        messages:
        - role: system
          template: '{memory}'
        - role: user
          template: '{text}'
      - id: answer
        type: chat_model
        model: chat
        inputs:
          messages:
            from: messages
        outputs:
          text: answer
      - id: conversation
        type: script
        language: starlark
        source: "def run(input):\n    return {\"turns\": list(input[\"messages\"]) + [{\"role\": \"assistant\"\
          , \"content\": input[\"answer\"]}]}\n"
        inputs:
          messages:
            from: input.messages
          answer:
            from: answer
        outputs:
          turns: turns
        limits:
          max_execution_steps: 1000
          timeout: 100ms
          max_input_bytes: 262144
          max_output_bytes: 262144
      - id: observe
        type: memory_observe
        turns_from: turns
        wait_for_completion: false
      edges:
      - from: start
        to: recall
      - from: recall
        to: prompt
      - from: prompt
        to: answer
      - from: answer
        to: conversation
      - from: conversation
        to: observe
      - from: observe
        to: end
      branches: []
      outputs:
      - node: answer
        field: answer
        name: assistant
        mime_type: text/plain
        primary: true
```

每个 binding 在 Registry 中按完整 key 协调构造。不同 key 可以独立进行网络工作；Registry 只在短临界区保留、发布或摘除 entry。每个 Workspace generation 持有可独立释放的 lease；shutdown 拒绝迟到的构造结果并等待在途工作退出，再关闭共享 backend。

`workspace` 将 Workspace ID 映射到 `Scope.AppID`；`peer` 将 owner Peer public key 派生为保留的 `peer:<hash>` AppID。User、Agent、Run 维度仍独立。Cloud/Volc 保留 provider 能表达的维度，自托管 Mem0 将完整 Scope 编码成 transport `user_id`。不同 Workspace 共享 Peer 记忆还必须指向同一 provider project 或同一自托管数据空间；相同 AppID 不会自动合并不同连接的数据。

删除 Workspace 只 purge 当前 binding 的 Workspace scope；Peer scope 在删除该 Peer 时 purge。handler 在 quiesce runtime 后解析 retained Workspace、Workflow alias 与 owner 当前 Profile，重复 purge 并检查为空后才 finalize。有残留返回 retryable `memory_residual`；provider 无法精确表达 scope 时以 terminal `memory_cleanup_unsupported` 停止。没有可解析的当前 binding 时无需清除，临时 provider/解析失败保持 retryable。清理不加载 extraction Model。GizClaw 不保留 binding 历史，不会访问以前连接下的数据；切换 driver、binding 或 scope 不迁移或删除旧数据。

## Ownership 与错误

Provider adapter 不关闭注入的 dependency。构造 workspace、index、HTTP client 或 credential dependency 的 composition root 拥有它们，并按构造顺序的逆序关闭资源。

稳定的 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 保留 `errors.Is` 语义。无法完整保持 filter、attribute patch 或 conditional-write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃条件。错误不得暴露 API key、access-key credential 或带 credential 的 response body。

物理 Memory Store 构造按完整 binding key 协调：相同 key 的调用者共享一个 backend，无关 binding 可以独立构造。Direct Mem0 Fact 幂等边界是完整 canonical Scope 加 observation ID，因此慢 provider 请求不会停止无关 scope/observation；相同 key 的 retry 仍会 reconcile 或返回 `ErrConflict`。
