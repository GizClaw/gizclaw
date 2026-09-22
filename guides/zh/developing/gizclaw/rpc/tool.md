# 设备过程调用

设备过程使用 `api/proto/rpc/payload/tool.proto` 中封闭的 `ClientTool` 注册表。Server 通过 `client.tool.v0.invoke` 调用一个已安装的过程；请求携带枚举值及该值指定的请求消息编码。`client.tool.v0.list` 返回设备实际安装的子集。`client.rpc.methods.list` 用数字 `RpcMethod` 报告协议族和版本。

`pkgs/gizclaw/peer_service_serve_peer_http_tool.go` 实现按 owner 限定的 HTTP 列表与调用路由。在打开 RPC stream 前验证类型化 JSON 参数，按 owner 串行化命令，按注册表解码响应并映射设备错误。RuntimeProfile Admin Tool 仍是 AI 和 Workflow runtime 使用的 Server 侧 HTTP 资源，与预定义设备过程独立。
