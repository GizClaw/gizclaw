# Labelers

`pkgs/genx/labelers` 根据当前查询选择用于 memory recall 的标签。它把自然语言查询转换为结构化 label matches，缩小后续检索范围。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers)

## 核心结构与主函数

| 符号 | 作用 |
| --- | --- |
| [`Labeler`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Labeler) | Query-time label selection contract。 |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Input) | 提供查询与候选标签信息。 |
| [`Match`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Match) | 表达被选中的标签及匹配信息。 |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Result) | 汇总 label matches。 |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#GenX) | 使用 Generator 选择 recall labels。 |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Process) | 选择 Labeler 并处理查询。 |

Labelers 不执行 vector search，也不管理标签的持久化生命周期；检索与存储由 Agent recall 和 stores 负责。
