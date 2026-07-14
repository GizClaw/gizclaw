# Peer HTTP · WebRTC

`实现文件：peer_service_webrtc.go`

实现 Peer HTTP 的 Giznet WebRTC Offer endpoint：将 typed API request 转交共享 signaling handler，再把 HTTP status、body 和错误转换成生成 API response。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `peerHTTP.CreateGiznetWebRTCOffer` | 接收 typed Offer request 并调用 Giznet signaling handler。 |
| `signalingResponseRecorder` | 捕获 signaling handler 写出的 status、headers 与 body。 |
| `createGiznetWebRTCOfferResponse` | 将 signaling HTTP 结果转换为生成 API response。 |
| `signalingErrorPayload` | 将 signaling error body 转换为稳定错误结构。 |
