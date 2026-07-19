# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) 定义 Agent runtime 可复用的长期记忆边界。它接收原始 observation，由 provider 提取并保存事实；读取侧返回 provider-neutral fact 和相关性分数。`genx.ModelContext` 是调用模型前的组合结果，不是这个 package 的存储模型。

## 核心边界

`Store` 提供四个操作：

- `Observe`：提交原始文本或带 role、speaker 和时间的 turns，触发事实提取；当前不支持非空的 turn attributes。
- `Recall`：按自然语言和结构化 filter 查询事实。
- `Update`：修改事实文本；支持 provider 时可同时 patch attributes 和校验 revision。
- `Delete`：删除或退休事实；支持 provider 时可校验 revision。

异步 provider 在 `ObserveResult.Operation` 返回状态。实现 `OperationWaiter` 的 store 由调用方使用已有 `context.Context` 等待完成，不在 constructor 中启动后台 goroutine。持久化 Flowcraft store 会在重启后从 canonical episode fact 恢复 operation ID；提取失败返回 terminal failed operation。

`AppID`、`UserID`、`AgentID` 和 `RunID` 是业务记忆 scope。它们不替代进程、credential 或远端服务自身的多租户隔离。Mem0 Platform 配置必须提供 API key，并至少设置其中一个 scope。Mem0 会把每个已配置 entity 作为独立的 memory layer；多 entity recall 使用 `OR`，不是层级交集。

## Provider

| Provider | 执行位置 | 持久化与模型配置 | 更新限制 |
| --- | --- | --- | --- |
| Flowcraft | 进程内 | `dir` 使用 Flowcraft workspace backend；模型资源名通过 `FlowcraftModelLoader` 解析 | append-only revision；支持文本、attributes 和 revision 校验，但 provider-owned fact 字段不能作为 metadata patch |
| Mem0 | 远端 HTTP | Platform 使用 `Authorization: Token`；self-hosted API key 使用 `X-API-Key`，模型由远端服务配置 | 支持文本更新；update/delete 会先确认目标至少匹配一个已配置 entity scope；不支持的 filter、attribute patch 或条件写入返回 `ErrUnsupported` |
| Volcengine AgentKit/Viking MEM0 | Volc control plane + Mem0 data plane | 必须配置项目 data-plane endpoint；可直接配置 Mem0 API key，也可使用 AK/SK 在必填的 memory project 中解析，并可选指定 API key ID | 与 Mem0 data plane 相同 |

Flowcraft 未配置 extraction model 时会把 observation 确定性地保存为 `note`。配置 extraction、embedding 或 rerank model 时，调用方必须向 `OpenFlowcraftStore` 或 `stores.NewWithStorageOptions` 提供 model loader。

Volc provider 不复制一套 fact CRUD 协议。它通过签名的 `DescribeMemoryProjectDetail` 和 `DescribeAPIKeyDetail` control-plane 调用解析 API key，然后复用 Mem0 client 执行 observation、recall、update、delete 和 event polling。`volc_memory.mem0.endpoint` 是必填项，避免 Volc credential 回退并发送到 Mem0 Platform 公共 endpoint。control plane 默认使用 `https://mem0.<region>.volcengineapi.com`，也可由 `control_endpoint` 覆盖。control-plane 解析必须提供 `memory_project_id`；可选的 `api_key_id` 用于在该项目内指定 key。未显式指定 key ID 时，resolver 只选择状态为 `Ready` 的 key。

## Server 配置

一个 logical memory store 必须只选择一个 provider：

```yaml
stores:
  agent-memory:
    kind: memory
    flowcraft:
      dir: ${GIZCLAW_MEMORY_DIR}
      runtime_id: detective-game
      user_id: player-42
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
      app_id: detective-game
      user_id: player-42
      agent_id: narrator
```

Volcengine AgentKit/Viking MEM0：

```yaml
stores:
  agent-memory:
    kind: memory
    volc_memory:
      mem0:
        endpoint: ${VOLC_MEM0_ENDPOINT}
        user_id: player-42
      memory_project_id: ${VOLC_MEMORY_PROJECT_ID}
      region: cn-beijing
      access_key_id: ${VOLC_ACCESS_KEY_ID}
      access_key_secret: ${VOLC_ACCESS_KEY_SECRET}
```

`cmd/internal/stores` 负责展开环境变量和管理 Flowcraft lifecycle。HTTP client、Volc credential resolver 和 Flowcraft model loader 都通过 `Options` 注入，测试和 Agent runtime 不需要修改公共 `Store` 契约。
显式配置的 Flowcraft `dir` 只能引用已设置且非空的环境变量，且展开后路径不能为空；否则 registry 构造返回 `ErrInvalidInput`，不会静默打开易失的进程内存储。
默认 server registry 不持有 model loader，因此会明确拒绝环境变量展开后非空的 Flowcraft model 字段；需要这些字段的 Agent runtime 必须使用 option-aware registry 路径。

Mem0 V3 search 只在 `filters` 内传递 entity ID。原生 entity、time、category 和 memory-ID 字段使用文档中的 operator；`FilterNotIn` 编码为包裹 `in` 的 `NOT`。其他 provider-neutral 字段表示自定义 metadata，只支持等值和不等值：Platform 将其放在 `metadata` 下，self-hosted Mem0 使用直接 metadata 字段 selector。无法精确映射的 operator 返回 `ErrUnsupported`。

## 错误语义

公共 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 必须保留 `errors.Is` 语义，不得在错误中返回 API key、AK/SK 或远端 response body 中的 credential。

不同 provider 无法保持某个 filter、attribute patch 或 conditional write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃请求条件。
在定义 provider-neutral 的 extraction context 映射前，per-turn attributes 同样遵循这一规则。
