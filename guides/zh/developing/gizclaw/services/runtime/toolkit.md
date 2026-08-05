# Tools

[Go API Reference](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolkit)

`toolkit` 负责 typed Tool Resource 的持久化、公共校验、防御性快照与
canonical-ID policy 过滤。Admin Tool resource 使用 caller-supplied、immutable
`metadata.id`；运行时执行名是显式的 immutable `spec.invoke_name`，不是第二个
Admin identity。RuntimeProfile binding 与 Admin `ToolkitPolicy.tool_ids` 保存
canonical ID。Peer RPC 把 binding key 投影为 scoped Tool `name`；Peer Toolkit
policy 和调用只使用该 scoped name，不暴露 canonical ID。

目前支持两种 Tool：

- `http_request` 声明一个固定 HTTPS `GET` 或 JSON `POST` 操作。参数通过
  RFC 6901 pointer 映射到 query 或 body field；status、response pointer、
  timeout 与 response size 都由 Resource 固定。
- `client_rpc` 调用当前已连接 Peer SDK 中按 canonical name 挂载的 handler。
  该分支没有 method、handler ID、Peer ID、endpoint 或 Credential 配置。

Resource contract 中不存在 `source`、`builtin`、executor registry、第二套 Tool
identity、`output_schema` 或 provider ToolCall ID。

## HTTP auth 与 transport

HTTP auth 是封闭 union：`none`、`bearer`、`header_api_key`、`volc_ark`、
`volc_search`、`volc_openapi`、`aliyun_app_code`、
`aliyun_openapi_v3`。Bearer token 与 header API key 是 write-only Resource
field：同一方法更新时省略 secret 会保留，提供新值会轮换，切换方法会删除旧
secret。Admin read、RuntimeProfile projection、model definition、日志与结果都
不会返回这些值。

Provider auth 在每次调用时解析一个 `volc` 或 `aliyun` Credential。Volc
Ark/Search 使用固定 API-key field；Volc OpenAPI 与阿里云 OpenAPI V3 对最终
request 签名；阿里云市场使用 AppCode。`pkgs/giztools` 只包含无状态执行 helper：
有界 HTTP request mapper/executor，以及针对当前 connection 的
`client.tool.invoke` wire client；它不解析 Resource、policy、RuntimeProfile，
不选择 Peer，也不实现 `genx.ToolInvoker`。

HTTP 仅允许 HTTPS，关闭 redirect 与环境 proxy；每次连接都检查全部 DNS 结果，
拒绝 private、loopback、link-local、multicast、unspecified、运营商 NAT 与
Server 配置的 denied network；同时校验 JSON status、content type、大小、语法和
response pointer。执行不会自动重试。

## Runtime 链路

```mermaid
flowchart LR
    Resource["Admin Tool canonical ID"] --> Profile["当前 Peer RuntimeProfile binding"]
    Profile --> Policy["Peer scoped Tool name"]
    Policy --> Invoker["context-scoped AgentHost ToolInvoker"]
    Invoker --> HTTP["http_request 走 giztools"]
    Invoker --> Client["client_rpc 走当前 Peer connection"]
    HTTP --> Continue["Transformer 或 Graph continuation"]
    Client --> Continue
```

Disabled Tool 不会被声明；dangling Resource 或同一 canonical ID 的重复 binding
会使 scope 构造失败。每次调用都会重新读取 Resource、重新授权、校验 model
arguments，再严格按 `spec.type` 分发；不会回退到另一类型、name、owner Profile
或其他在线 Peer。

Client 的 `timeout` 与 `unavailable` 会成为有界 JSON Tool result，交回模型继续
执行；原始 handler、transport、Peer 与 Credential 信息会被隐藏。ToolCall 与
ToolResult 始终是 Transformer/Graph 内部控制，不会作为 public assistant stream
control message 发给 Peer。
