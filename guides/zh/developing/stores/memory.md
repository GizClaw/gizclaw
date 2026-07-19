# Memory Store

[`pkgs/store/memory`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/store/memory) 定义 Agent runtime 可复用的长期记忆边界。它接收原始 observation，由 provider 提取并保存事实；读取侧返回 provider-neutral fact 和相关性分数。`genx.ModelContext` 是调用模型前的组合结果，不是这个 package 的存储模型。

## 核心边界

`Store` 提供四个操作：

- `Observe`：提交原始文本或带 role、speaker、时间和 attributes 的 turns，触发事实提取。
- `Recall`：按自然语言和结构化 filter 查询事实。
- `Update`：修改事实文本；支持 provider 时可同时 patch attributes 和校验 revision。
- `Delete`：删除或退休事实；支持 provider 时可校验 revision。

异步 provider 在 `ObserveResult.Operation` 返回状态。实现 `OperationWaiter` 的 store 由调用方使用已有 `context.Context` 等待完成，不在 constructor 中启动后台 goroutine。

`AppID`、`UserID`、`AgentID` 和 `RunID` 是业务记忆 scope。它们不替代进程、credential 或远端服务自身的多租户隔离。

## Provider

| Provider | 执行位置 | 持久化与模型配置 | 更新限制 |
| --- | --- | --- | --- |
| Flowcraft | 进程内 | `dir` 使用 Flowcraft workspace backend；模型资源名通过 `FlowcraftModelLoader` 解析 | append-only revision；支持文本、attributes 和 revision 校验 |
| Mem0 | 远端 HTTP | Platform 只需 endpoint/API key；self-hosted 由远端服务配置模型 | 支持文本更新；远端 API 不提供 attribute patch 或条件写入时返回 `ErrUnsupported` |
| Volcengine AgentKit/Viking MEM0 | Volc control plane + Mem0 data plane | 可直接配置 Mem0 API key，也可使用 AK/SK 和 API key ID 或 memory project ID 解析 | 与 Mem0 data plane 相同 |

Flowcraft 未配置 extraction model 时会把 observation 确定性地保存为 `note`。配置 extraction、embedding 或 rerank model 时，调用方必须向 `OpenFlowcraftStore` 或 `stores.NewWithStorageOptions` 提供 model loader。

Volc provider 不复制一套 fact CRUD 协议。它通过签名的 `DescribeMemoryProjectDetail` 和 `DescribeAPIKeyDetail` control-plane 调用解析 API key，然后复用 Mem0 client 执行 observation、recall、update、delete 和 event polling。

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

## 错误语义

公共 sentinel errors 是 `ErrInvalidInput`、`ErrNotFound`、`ErrUnsupported`、`ErrConflict` 和 `ErrUnavailable`。Provider 必须保留 `errors.Is` 语义，不得在错误中返回 API key、AK/SK 或远端 response body 中的 credential。

不同 provider 无法保持某个 filter、attribute patch 或 conditional write 语义时，必须返回 `ErrUnsupported`，不能静默丢弃请求条件。
