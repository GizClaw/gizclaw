# Admin HTTP · Resources

源码：[peer_service_serve_admin.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin.go)

启动 Admin HTTP service，实现 declarative resource 的 apply、get、put 和 delete，校验 URL 中的 kind/name 与资源一致，并统一映射 resource manager 错误。

具体资源 ownership 仍位于各领域；该文件只提供统一 Admin resource surface。
