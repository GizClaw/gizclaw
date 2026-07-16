# Segmentors

`pkgs/genx/segmentors` Organize conversation content into segments, entities and relations, providing structured results for memory writing and subsequent recall.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Segmentor`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Segmentor) | Conversation segmentation contract. |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Input) | Carrying the conversation input to be analyzed. |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Result) | Return segments, entities and relations. |
| [`Schema`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Schema) | Constrain the entities and relationship structures that can be extracted. |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#GenX) | Use Generator to complete structured segmentation. |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/segmentors#Process) | Select the Segmentor via the default mux and process the input. |

Segmentors are only responsible for structuring content and do not save conversation, entity graph or vector index; these persistence responsibilities belong to Agent memory and stores.
