# Admin HTTP · Logs

Source：[peer_service_serve_admin_logs.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin_logs.go)

Implement Admin SSE stream for Server log: parse filter conditions, handle first events and streaming errors, and encode SSE events.

Log query contract is located in Server Log Query; the actual log backend is provided by the host.
