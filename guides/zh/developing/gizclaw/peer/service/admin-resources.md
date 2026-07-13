# Admin HTTP · Resources

`实现文件：peer_service_serve_admin.go`

启动 Admin HTTP service，实现 declarative resource 的 apply、get、put 和 delete，校验 URL 中的 kind/name 与资源一致，并统一映射 resource manager 错误。

具体资源 ownership 仍位于各领域；该文件只提供统一 Admin resource surface。

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| `adminService` | 聚合 Admin HTTP 所需的 resource manager 与领域 services。 |
| `serveAdmin` | 在 Admin Giznet service 上启动 generated Admin HTTP server。 |
| `ApplyResource` | 按 declarative resource contract 创建或更新资源。 |
| `GetResource` / `PutResource` / `DeleteResource` | 实现通用资源读取、替换和删除。 |
| `validateResourcePathMatch` | 校验 request path 与 resource body 的 kind/name 一致。 |
| `resourceManagerError` | 将 resource manager error 映射成 HTTP status 与 API error。 |
