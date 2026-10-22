# Peer HTTP · WebRTC

源码：[peer_service_webrtc.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_webrtc.go)

实现 Peer HTTP 的 Giznet WebRTC Offer endpoint：将 typed API request 转交共享 signaling handler，再把 HTTP status、body 和错误转换成生成 API response。
