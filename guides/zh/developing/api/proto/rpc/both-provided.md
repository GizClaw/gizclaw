# Both Provided

Both Provided RPC 由连接两端都实现。Client 或 Server 均可作为 caller，也均可作为 provider，用于连接诊断和 transport measurement，不读取某一端独有的产品资源。

## Methods

| Method | 作用 | 实现要求 |
| --- | --- | --- |
| `all.ping` | 验证 RPC request/response path 可用 | Client 与 Server 都返回 Ping response |
| `all.speed_test.run` | 在 RPC stream 上执行吞吐测试 | Client 与 Server 都处理 speed-test stream |

## 调用关系

```mermaid
sequenceDiagram
    participant A as Client or Server
    participant B as Server or Client
    A->>B: all.ping / all.speed_test.run
    B-->>A: response / stream frames
```

`all.*` 只适合真正对称的基础能力。只由一端拥有的数据或行为必须使用 `client.*` 或 `server.*`，不能为了复用 handler 放入 `all.*`。
