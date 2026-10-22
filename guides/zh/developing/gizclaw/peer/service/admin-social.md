# Admin HTTP · Social

源码：[peer_service_serve_admin_social.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin_social.go)

实现 contact、friend、Peer friend、friend group、member 和 invite token 的 Admin endpoints；同时通过 Peer RPC 读取 workspace history 列表、详情和音频。

Social graph 属于 `services/social`；workspace history 属于 AI/runtime workspace service。
