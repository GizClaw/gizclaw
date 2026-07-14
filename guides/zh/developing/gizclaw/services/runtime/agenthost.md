# Agent Host

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost)

`agenthost` 拥有 Agent instance 的在线生命周期。它解析运行规格、取得 workspace lease、建立输入输出 Stream、接入 history 与 ToolKit，并维护当前 runtime registry。

## 运行流程

```mermaid
flowchart TD
    Reload["Service.Reload"] --> Resolve["Resolver.Resolve"]
    Resolve --> Lease["Coordinator.Acquire"]
    Lease --> Agent["Host.NewAgent"]
    Agent --> Input["StreamSource"]
    Agent --> Output["StreamConsumer"]
    Output --> History["Workspace history / audio output"]
    Agent --> Toolkit["ToolkitContext"]
    Input --> Stop["Stop / cancel / release"]
    Output --> Stop
```

## 核心结构与主函数

| 结构或函数 | 作用 |
| --- | --- |
| `Service.Reload` | 停止旧 runtime，并按当前 Peer run selection 创建新 runtime。 |
| `Service.Status` / `Stop` | 查询或终止当前 Agent runtime。 |
| `Service.WorkspaceState` | 返回当前 workspace 的运行状态。 |
| `RuntimeRegistry` | 维护当前在线 runtime。 |
| `Coordinator` / `MemoryCoordinator` | 为 workspace 提供排他 lease。 |
| `Host` / `Registry` | 根据解析后的 `Spec` 选择并创建 Agent。 |
| `InputStream` / `PushSource` | 将连续输入转换为 Agent 消费的 GenX Stream。 |
| `MixerOutput` | 将 Agent audio output 接到 mixer track。 |
| `ToolkitContext` | 为一次 runtime 组合授权后的 ToolKit。 |

所有 runtime 创建路径都必须具有对称的 cancel、stream close、lease release 和 registry cleanup。Agent definition、Workflow 与 Workspace 的持久化仍属于 AI services。
