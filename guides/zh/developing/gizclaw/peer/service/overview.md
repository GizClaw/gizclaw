# Services

Peer Services 将 GizClaw 产品能力暴露到一条 Peer connection 可以打开的 Giznet service。入口按实际职责继续拆分：

| 职责 | 实现文件 |
| --- | --- |
| [Core Service](./core) | `peer_service.go` |
| [Peer HTTP · WebRTC](./webrtc) | `peer_service_webrtc.go` |
| [HTTP Service Entrypoints](./public-http) | `peer_service_serve_peer_http.go` |
| [Peer HTTP · /me](./peer-http-me) | `peer_service_serve_peer_http_self.go` |
| [Peer HTTP · Side Control](./side-control) | `peer_service_serve_peer_http_side_control.go` |
| [Admin HTTP · Resources](./admin-resources) | `peer_service_serve_admin.go` |
| [Admin HTTP · ACL](./admin-acl) | `peer_service_serve_admin_acl.go` |
| [Admin HTTP · Gameplay](./admin-gameplay) | `peer_service_serve_admin_gameplay.go` |
| [Admin HTTP · Logs](./admin-logs) | `peer_service_serve_admin_logs.go` |
| [Admin HTTP · Social](./admin-social) | `peer_service_serve_admin_social.go` |
| [Admin HTTP · Telemetry](./admin-telemetry) | `peer_service_serve_admin_telemetry.go` |

这些文件是 public surface 到已有领域 service 的适配层。资源、storage、validation 与业务生命周期仍由相应的 `services/<domain>` 拥有。
