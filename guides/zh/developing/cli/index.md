# CLI <Badge type="warning" text="WIP" />

> 本页目前只定义 CLI 的目录与 ownership，命令树、配置、连接和运行流程仍待逐项补充。

`cmd/gizclaw` 是 GizClaw CLI 的 executable 入口，`cmd/internal` 保存 CLI 组装、命令、连接、日志、路径、服务和本地 store 等内部实现。

CLI 负责把用户输入和本地环境转换为稳定的 SDK 或 service 调用，不拥有 GizClaw 领域资源、RPC contract 或 transport implementation。可复用 client 能力应进入对应 SDK；服务端业务行为应进入 `pkgs/gizclaw`。

## 目录

```text
cmd/
├── gizclaw/          # executable main
└── internal/
    ├── commands/     # command tree 与各命令入口
    ├── connection/   # CLI connection orchestration
    ├── adminapi/     # Admin API adapter
    ├── deviceapi/    # Device-facing adapter
    ├── peerapi/      # Peer-facing adapter
    ├── server/       # 本地 server wiring
    ├── service/      # CLI service wiring
    ├── storage/      # CLI-owned local state
    └── stores/       # CLI store construction
```

修改 CLI 调用的 API 或 RPC 时，应同时阅读对应 [API Design](../api/overview) 和 SDK 文档。
