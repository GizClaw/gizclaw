# Labelers

`pkgs/genx/labelers` Selects tags for memory recall based on the current query. It converts natural language queries into structured label matches, narrowing the scope of subsequent searches.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Labeler`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Labeler) | Query-time label selection contract. |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Input) | Provide query and candidate label information. |
| [`Match`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Match) | Express the selected label and matching information. |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Result) | Summarize label matches. |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#GenX) | Use Generator to select recall labels. |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/labelers#Process) | Select Labeler and process the query. |

Labelers do not perform vector search and do not manage the persistence life cycle of labels; retrieval and storage are handled by Agent recall and stores.
