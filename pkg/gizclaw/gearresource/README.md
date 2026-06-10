# Gear Resource

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/19

This package is reserved for Gear Service RPCs that expose existing resource
domains to a gear through ACL-controlled resource APIs.

Planned scope:

- `workspace.{list,get,create,put,delete}`
- `workflow.{list,get,create,put,delete}`
- `model.{list,get,create,put,delete}`
- `credential.{list,get,create,put,delete}`

Domain storage remains in the existing workspace, workflow, model, and
credential packages.
