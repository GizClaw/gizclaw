# Peer HTTP · /me

`实现文件：peer_service_serve_peer_http_self.go`

这是 `ServicePeerHTTP` 中以 `/me` 为根路径的 endpoints，负责读取或更新调用者自己的 Peer resource、status 与 runtime，并校验调用者只能操作自己的 Peer identity。

Peer 持久化资源和 runtime 状态分别由 `services/runtime/peer`、`peerrun` 和相关 runtime service 拥有。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerHTTP.GetMe` | 返回调用者自己的 Peer resource。 |
| `peerHTTP.GetMeStatus` | 返回调用者自己的在线 status。 |
| `peerHTTP.PutMeStatus` | 更新调用者自己的 status。 |
| `peerHTTP.GetMeRuntime` | 返回调用者自己的 runtime view。 |
| `ensurePeerHTTPCaller` | 校验 URL/请求中的 Peer 与 session caller identity 一致。 |
