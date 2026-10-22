# Profilers

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/profilers)

`pkgs/genx/profilers` 根据新的 conversation segment 更新实体画像，输出画像内容及 schema change，供 Agent memory 决定如何持久化。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/profilers)

Profiler 不拥有 entity identity、graph 或 profile storage；它只生成可由 Agent memory 应用的结果。
