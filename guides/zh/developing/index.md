# 开发指引

开发指引按照仓库目录组织。每个目录章节说明它负责什么、包含哪些子目录、依赖谁、什么代码应该写在这里，以及哪些内容不属于这里。

## Packages

- [`pkgs/giznet`](giznet.md)：与 GizClaw 业务无关的连接与传输 contract，以及 WebRTC 和 HTTP-over-service-stream 实现。
- [`pkgs/gizclaw`](gizclaw/overview.md)：GizClaw 产品 contract、领域服务、peer runtime 和 Server 组装。
- [`pkgs/gizedge`](gizedge.md)：面向公网的 Edge ingress、上游 Server 转发和可选 TURN runtime。
