# HTTP Service Entrypoints

`实现文件：peer_service_serve_peer_http.go`

提供普通 Peer Public HTTP 与 Edge Public HTTP，组装 API Key、CORS、OpenAI API 和 Edge signaling routes，并执行 Edge client/signaling Peer 的准入判断。

该文件拥有 HTTP surface composition；API Key 状态属于 `services/system/apikey`，具体 API 行为属于对应领域 service。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `servePublic` / `serveEdgePublic` | 在对应 Giznet service 上启动普通或 Edge Public HTTP。 |
| `publicHTTPHandlerWithOptions` | 组装 API Key 管理与 signaling routes。 |
| `allowEdgeClientPeer` | 判断 Peer 是否允许作为 Edge client。 |
| `allowEdgeSignalingPeer` | 判断 Peer 是否允许通过 Edge 发起 signaling。 |
| `setPeerHTTPCORSHeaders` | 设置 Peer HTTP surface 的 CORS headers。 |
