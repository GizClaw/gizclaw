# Labelers

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/labelers)

`pkgs/genx/labelers` 根据当前查询选择用于 memory recall 的标签。它把自然语言查询转换为结构化 label matches，缩小后续检索范围。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/labelers)

Labelers 不执行 vector search，也不管理标签的持久化生命周期；检索与存储由 Agent recall 和 stores 负责。
