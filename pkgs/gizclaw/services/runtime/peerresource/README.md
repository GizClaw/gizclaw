# Peer Resource

This package implements Peer RPC methods that expose existing resource domains
through RuntimeProfile-, owner-, and domain-controlled resource APIs.

Supported resource surfaces include:

- `server.workspace.{list,get,create,put,delete}`
- `server.workflow.{list,get}`
- `server.model.{list,get,create,put,delete}`
- `server.credential.{list,get,create,put,delete}`

Domain storage remains in the owning workspace, workflow, model, credential,
firmware, gameplay, and social packages.
