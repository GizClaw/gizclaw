# DashScope Adapter

DashScope Adapter adapts a DashScope realtime multimodal session to `genx.Transformer` through `dashscoperealtime.Transformer`.

The public constructor is `dashscoperealtime.New(dashscoperealtime.Config{Client: client})`. It stores immutable provider options without opening a WebSocket; each concurrent `Transform` call creates its own session.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `dashscoperealtime.Config` | Stores the client, realtime model, audio format, voice, instructions, and turn detection configuration. |
| `dashscoperealtime.New` | Creates a Transformer from typed Config without opening a connection. |
| `dashscoperealtime.Transformer.Transform` | Establishes an independent realtime session for each call, writes the input Stream to the provider, and returns the unified output Stream. |
| `dashscoperealtime.Stream` | Wraps the realtime output Stream that supports session updates. |

Provider session update and event name remain inside the Adapter; the caller only relies on GenX Stream and an explicit update contract.
