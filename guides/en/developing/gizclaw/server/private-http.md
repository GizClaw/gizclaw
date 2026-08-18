# Public client API switch

`CmdServer.ServeHTTP` enforces `serve-to-clients` before bearer authentication. When disabled, `/gizclaw/v1/*` and `/openai/v1/*` return `403 PRIVATE_INGRESS_DENIED`; `/server-info` and WebRTC signaling remain available for connection establishment. There is no private-session bypass.
