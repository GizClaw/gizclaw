# Admin API

The Admin API targets operators, CLI, and admin UI with administrative privileges. It is responsible for declarative resource management, Peer management, Telemetry query and server operation and maintenance, and is not used by ordinary peers as product data channels.

Source:`api/http/admin.json`
Go generated output: `pkgs/gizclaw/api/adminhttp`

## Surface grouping

| Grouping | Main Responsibilities |
| --- | --- |
| Resource | `apply/show` and unified Resource envelope |
| Peer | Peer query, approval, blocking, refresh, configuration and runtime |
| Runtime access | RuntimeProfile and RegistrationToken management |
| AI | Credential, Model, Voice, Provider Tenant, Workflow, Workspace |
| Gameplay | Game Rule, Pet, Badge, Points, Result and Reward |
| Social | Contact, Friend and Friend Group Management |
| Firmware | Firmware resource, release, rollback and artifact |
| Observability | Server log stream and Peer telemetry query |

Admin OpenAPI only has HTTP path, request/response and wire error. Resource validation, authorization, storage and domain lifecycle are implemented by corresponding services and resource managers.

## Resource dependency

Admin quotes `shared.json`; the generation entry continues to quote `resources/*.json`:

```text
shared/ ← resources/ ← shared.json ← admin.json
```

Resource-specific Spec and Resource are placed in the same file; the Admin API should not load the entire Resource graph indirectly through `shared.json`.
