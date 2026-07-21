# Peer Resource

This package implements Peer RPC methods that expose existing resource domains
through RuntimeProfile- and domain-controlled resource APIs.

Supported resource surfaces include:

- `server.workspace.{list,get,create,put,delete}`
- `server.workflow.{list,get}`
- `server.model.{list,get}`
- `server.voice.{list,get}`
- `server.tool.{list,get}`

Workflow, Model, Voice, Tool, Credential, and gameplay definitions are created
through the Admin API. Peer reads expose only aliases selected by the active
RuntimeProfile; canonical resource IDs and Credential resources are not part of
the Peer surface. Workspace and adoption state remain Peer-created runtime
state.

Domain storage remains in the workspace, workflow, model, credential, firmware,
gameplay, and social packages.
