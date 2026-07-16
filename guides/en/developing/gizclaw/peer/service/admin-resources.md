# Admin HTTP · Resources

`Implementation file: peer_service_serve_admin.go`

Start the Admin HTTP service, implement apply, get, put and delete of declarative resources, verify that the kind/name in the URL is consistent with the resource, and uniformly map resource manager errors.

Specific resource ownership still resides in each realm; this document only provides a unified Admin resource surface.

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `adminService` | Aggregates the resource manager and domain services required by Admin HTTP. |
| `serveAdmin` | Start the generated Admin HTTP server on the Admin Giznet service. |
| `ApplyResource` | Create or update resources according to declarative resource contract. |
| `GetResource` / `PutResource` / `DeleteResource` | Implement universal resource reading, replacement and deletion. |
| `validateResourcePathMatch` | Verify that the request path is consistent with the kind/name of the resource body. |
| `resourceManagerError` | Map resource manager error to HTTP status and API error. |
