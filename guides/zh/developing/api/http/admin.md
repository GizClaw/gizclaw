# Admin API

Admin API 面向获得管理权限的 operator、CLI 和管理 UI。它负责声明式资源管理、Peer 管理、Telemetry 查询和 Server 运维，不供普通 Peer 作为产品数据通道使用。

Source：`api/http/admin.json`
Go 生成输出：`pkgs/gizclaw/api/adminhttp`

## Surface 分组

| 分组 | 主要职责 |
| --- | --- |
| Resource | `apply/show` 及统一 Resource envelope |
| Peer | Peer 查询、批准、阻止、刷新、配置与 runtime |
| Runtime access | RuntimeProfile 与 RegistrationToken 管理 |
| AI | Credential、Model、Voice、Provider Tenant、Workflow、Workspace |
| Gameplay | Game Rule、Pet、Badge、Points、Result 与 Reward |
| Social | Contact、Friend 与 Friend Group 管理 |
| Firmware | Firmware resource、release、rollback 与 artifact |
| Observability | Server log stream 与 Peer telemetry query |

Admin OpenAPI 只拥有 HTTP path、request/response 和 wire error。Resource validation、authorization、storage 和领域 lifecycle 由对应 services 与 resource manager 实现。

## Resource 依赖

Admin 引用 `shared.json`；该生成入口继续引用 `resources/*.json`：

```text
shared/ ← resources/ ← shared.json ← admin.json
```

Resource 专属 Spec 与 Resource 放在同一文件；Admin API 不应通过 `shared.json` 间接加载整个 Resource graph。
