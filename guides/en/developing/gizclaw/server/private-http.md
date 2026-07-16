# Private HTTP

`Implementation file: server_private_http.go`

Verify the session headers of the private HTTP ingress, perform ingress authorization based on the caller's public key, and construct a session authorizer for the public login.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `ErrPrivateHTTPIngressDenied` | Private ingress authorization rejection error. |
| `Server.AuthenticateHTTPSessionHeaders` | Verify session identity from Authorization and public-key headers. |
| `Server.AuthorizePrivateHTTPIngress` | Determine whether the specified Peer is allowed to access private HTTP ingress. |
| `PrivateHTTPIngressLoginAuthorizer` | Adapt Server ingress authorization to public-login authorizer. |
