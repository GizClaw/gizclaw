# Admin CLI

## User Story

As an admin operator, I want `gizclaw admin` commands to inspect and manage server resources through real CLI invocations.

## Covered Behaviors

- Admin-capable contexts can list and inspect peers.
- Credential, tenant, voice, workflow, workspace, model, firmware, and declarative resource commands round-trip against the server.
- File-based apply/upload commands use real filesystem inputs.
- Provider-specific validation failures surface as user-facing CLI errors.
- Batch resource show returns ordered full resources for mixed kinds and duplicate references; partial failure preserves successful entries, emits null for missing resources, and exits nonzero with indexed diagnostics.
