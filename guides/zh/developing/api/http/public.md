# Public API

Public API 是 Server 在 WebRTC connection 建立前后向 Public/Peer caller 暴露的 HTTP contract。它是入口边界，不代表 Peer 领域 service 的全部能力。

Source：`api/http/peer.json`
Go 生成输出：`pkgs/gizclaw/api/peerhttp`

## Endpoints

| Endpoint | 作用 |
| --- | --- |
| `POST /login` | 建立 Primary session，或使用 device token 建立 Side Control session |
| `GET /server-info` | 查询 Server public information |
| `POST /webrtc/v1/offer` | 提交签名 Offer 并取得 WebRTC Answer |
| `GET /me` | 查询当前 Peer registration/self view |
| `GET /me/status` | 查询当前 Peer status |
| `PUT /me/status` | 更新当前 Peer status |
| `GET /me/runtime` | 查询当前 Peer runtime |
| `/me/side-control/*` | Primary device token 与 Side Control session 管理 |
| `/side-control/*` | 查询或控制 Side Control session 绑定的目标设备 |

`/webrtc/v1/offer` 发生在 Peer connection 建立之前，必须保留 HTTP signaling。建立连接后的 Peer 能力可以使用 reliable HTTP-over-service-stream 或 Peer RPC；选择 transport 时应避免为相同能力维护两套 contract。

Offer 的身份认证由签名 signaling contract 自身完成，不应额外依赖 Public login session。Public API 可以复用 `ErrorResponse`、`DeviceInfo` 和 `Runtime` 等真正 shared 类型，但不引用 Admin Resources。

Side Control 的 route contract、session 边界与 transport 见 [Peer HTTP · Side Control](../../gizclaw/peer/service/side-control)。设备密码、Wi-Fi、配网和播放声音等 LiteLink 本地能力不属于 Public API。
