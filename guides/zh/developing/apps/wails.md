# Wails App <Badge type="warning" text="WIP" />

> 本页目前只定义 Desktop App 的总体边界，Go bridge、frontend 模块和运行流程仍待逐项补充。

`apps/wails` 是 GizClaw desktop application。Go backend 负责 native/desktop integration 与 Wails bridge，`frontend` 负责用户界面和浏览器侧状态。

```text
apps/wails/
├── internal/
│   ├── appconfig/    # Desktop app configuration
│   └── bridge/       # Go 与 frontend 的 bridge
├── frontend/         # Web frontend、测试与构建配置
└── build/            # Wails platform build metadata
```

Desktop App 不应复制 `pkgs/gizclaw` 的服务端业务，也不应绕过生成 client 手写已有 API。Go bridge 只暴露 UI 所需的稳定 application capability，并负责 native resource lifecycle。

Go 部分遵循 [Go 编码规范](/zh/coding-styles/go)，frontend 遵循 [JavaScript 与 TypeScript](/zh/coding-styles/js)。
