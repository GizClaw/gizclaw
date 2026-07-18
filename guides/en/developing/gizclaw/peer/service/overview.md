# Services

Peer Services exposes GizClaw product capabilities to a Giznet service that can be opened by a Peer connection. The entry point continues to be split according to actual responsibilities:

| Responsibilities | Implementation Documents |
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

These files are the adaptation layer from public surfaces to existing domain services. Resources, storage, validation and business lifecycle are still owned by the corresponding `services/<domain>`.
