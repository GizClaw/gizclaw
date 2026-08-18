# OpenAI HTTP

`实现文件：server_openai_http.go`

为普通 Server HTTP 入口组装 Peer-scoped OpenAI-compatible handler，并把 public-login session 接入对应 RuntimeProfile resource view。

Server 在 strip `/openai` 前验证 primary session。`PeerService` 中保留的 handler 随后执行 exact method/path allowlist，绑定已验证的 canonical Peer ID 与 request-scoped resources，并把四个标准 operation 交给 AI Server Shell。`/v1/voices` 继续作为 Shell 外的 GizClaw handler。Unsupported path 只会在 ingress authentication 后返回 `404`。

Bearer 与 cookie value 不作为 backend identity。Shell authenticator 只读取 verified context binding，缺失时 fail closed。CORS 与 preflight behavior 继续由外层 Server handler 拥有。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerOpenAIHTTPHandler` | 验证 primary session，并在 dispatch 前绑定其 immutable registration snapshot。 |
| `openAIProtocolHandler` | 为一个 `PeerService` handler graph 懒构建并保留单个 Shell router，避免无关的 Server 启动等待 OpenAI Schema 解析。 |
