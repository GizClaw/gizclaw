# Labelers

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/genx/labelers)

`pkgs/genx/labelers` Selects tags for memory recall based on the current query. It converts natural language queries into structured label matches, narrowing the scope of subsequent searches.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/genx/labelers)

Labelers do not perform vector search and do not manage the persistence life cycle of labels; retrieval and storage are handled by Agent recall and stores.
