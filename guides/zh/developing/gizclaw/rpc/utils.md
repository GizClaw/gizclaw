# Utilities

源码：[rpc_utils.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/rpc_utils.go)

提供 RPC runtime 共用的单请求 dispatch、stream request 处理、Ping、client call、request/result 构造、payload validation、API error mapping 和类型转换 helper。

该文件是 RPC 内部辅助实现，不拥有独立领域能力。
