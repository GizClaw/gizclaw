# Segmentors

[源码](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/segmentors)

`pkgs/genx/segmentors` 将 conversation 内容整理为 segment、entity 和 relation，为 memory 写入与后续 recall 提供结构化结果。

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/segmentors)

Segmentors 只负责内容结构化，不保存 conversation、entity graph 或 vector index；这些持久化职责属于 Agent memory 与 stores。
