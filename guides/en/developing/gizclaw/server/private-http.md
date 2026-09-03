# Edge-only public client APIs

`CmdServer.ServeHTTP` always rejects direct `/gizclaw/v1/*` and `/openai/v1/*` requests with `403 PRIVATE_INGRESS_DENIED` before bearer authentication. `/server-info` and WebRTC signaling remain available for connection establishment. Edge reaches the business handlers through `ServiceEdgeHTTP`; there is no direct-HTTP or private-session bypass.
