# Profilers

`pkgs/genx/profilers` 根据新的 conversation segment 更新实体画像，输出画像内容及 schema change，供 Agent memory 决定如何持久化。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Profiler`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Profiler) | Entity profile 更新 contract。 |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Input) | 提供现有 profile 与新增内容。 |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Result) | 返回更新后的 profile 与变更信息。 |
| [`SchemaChange`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#SchemaChange) | 描述画像 schema 的增量变化。 |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#GenX) | 使用 Generator 生成 profile update。 |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Process) | 选择 Profiler 并处理画像更新。 |

Profiler 不拥有 entity identity、graph 或 profile storage；它只生成可由 Agent memory 应用的结果。
