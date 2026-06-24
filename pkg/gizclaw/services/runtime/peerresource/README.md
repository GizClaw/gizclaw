# Peer Resource

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/19

This package is reserved for Server Service RPCs that expose existing resource
domains to a peer through ACL-controlled resource APIs.

Planned scope:

- `server.workspace.{list,get,create,put,delete}`
- `server.workflow.{list,get,create,put,delete}`
- `server.model.{list,get,create,put,delete}`
- `server.credential.{list,get,create,put,delete}`

Domain storage remains in the existing workspace, workflow, model, and
credential packages.
