# Admin HTTP · Resources

Source：[peer_service_serve_admin.go](https://github.com/GizClaw/gizclaw/blob/main/pkgs/gizclaw/peer_service_serve_admin.go)

Start the Admin HTTP service, implement apply, get, put and delete of declarative resources, verify that the kind/name in the URL is consistent with the resource, and uniformly map resource manager errors.

Specific resource ownership still resides in each realm; this document only provides a unified Admin resource surface.
