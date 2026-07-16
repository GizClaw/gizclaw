# Agent Host

`Implementation file: peer_agent_host.go`

| Documentation | Features included |
| --- | --- |
| `peer_agent_host.go` | Create an Agent Host dedicated to the current Peer based on the common `agenthost.Host` and connect to the Peer-backed GenX provider. |

This file is only responsible for Host wiring on the Peer connection. Agent instance, input and output, history, toolkit and running life cycle belong to `services/runtime/agenthost`.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `newPeerAgentHost` | Create a Peer-scoped Agent Host based on the general Host and install the Peer GenX provider. |
