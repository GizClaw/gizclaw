# Common RPC

`Implementation file: rpc_all.go`

Implement the Ping call common to all RPC connections, and connect the request ID, Ping payload and response decoding to the common RPC call path.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `rpcServer.Ping` | Construct Ping request, call and decode the response by specifying RPC connection. |
| [`PeerConn.Ping`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#PeerConn.Ping) | Open an RPC stream for the Peer connection, perform Ping and close the stream. |
