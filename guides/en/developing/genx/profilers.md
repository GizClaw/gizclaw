# Profilers

`pkgs/genx/profilers` Update the entity portrait according to the new conversation segment, output the portrait content and schema change, and let the Agent memory decide how to persist.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`Profiler`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Profiler) | Entity profile update contract. |
| [`Input`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Input) | Provide existing profiles and new content. |
| [`Result`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Result) | Return the updated profile and change information. |
| [`SchemaChange`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#SchemaChange) | Describes incremental changes to the profile schema. |
| [`GenX`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#GenX) | Use Generator to generate profile update. |
| [`Process`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/profilers#Process) | Select Profiler and process profile updates. |

The Profiler does not own an entity identity, graph, or profile storage; it only produces results that can be applied by Agent memory.
