# DashScope Adapter

DashScope Adapter adapts DashScope realtime multimodal session to `genx.Transformer` through `DashScopeRealtime`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`DashScopeRealtime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#DashScopeRealtime) | Stores realtime model, audio format, voice, instructions and turn detection configurations. |
| [`NewDashScopeRealtime`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#NewDashScopeRealtime) | Create Transformer using DashScope client. |
| `DashScopeRealtime.Transform` | Establish a realtime session, write the input Stream to the provider, and return the unified output Stream. |
| [`DashScopeStream`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#DashScopeStream) | Wraps the realtime output Stream that supports session update. |

Provider session update and event name remain inside the Adapter; the caller only relies on GenX Stream and an explicit update contract.
