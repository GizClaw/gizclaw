# Security Policy

`Implementation file: server_security_policy.go`

Implement the transport security policy of Giznet Server: determine whether the public key allows the establishment of a Peer connection, and whether the Peer is allowed to open the specified Giznet service.

It owns connection and service admission; RuntimeProfile, ownership, and domain relationships decide product-resource access.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| [`ServerSecurityPolicy`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy) | Adapt the complete Server configuration to the Giznet security policy. |
| [`ServerSecurityPolicy.AllowPeer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowPeer) | Determine whether the public key allows connection establishment. |
| [`ServerSecurityPolicy.AllowService`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/gizclaw#ServerSecurityPolicy.AllowService) | Determine service access based on Peer identity and service ID. |
