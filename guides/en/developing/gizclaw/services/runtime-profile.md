# RuntimeProfile and device registration

`RuntimeProfile` is the connection-scoped environment exposed to a device. Administrators create canonical Workflow, Model, Voice, Tool, PetDef, GameDef, BadgeDef, and Path resources; a Peer cannot create those resources. A Peer may create Workspace state and adopt Pet instances.

## Declarative structure

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  name: default
spec:
  workflows:
    collections:
      assistants:
        doubao-realtime:
          resource_id: doubao-realtime-conversation
          i18n:
            en: {display_name: Doubao Assistant}
            zh-CN: {display_name: 豆包助手}
      raids:
        journey:
          resource_id: flowcraft-journey-guide
          i18n:
            en: {display_name: Journey Guide}
            zh-CN: {display_name: 旅途向导}
  resources:
    models:
      asr:
        resource_id: volc-bigasr-sauc
        i18n:
          en: {display_name: Speech Recognition}
          zh-CN: {display_name: 语音识别}
    voices:
      assistant:
        resource_id: volc-tenant:volc-main:zh_female_shaoergushi_mars_bigtts
        i18n:
          en: {display_name: Assistant}
          zh-CN: {display_name: 助手}
    pet_defs:
      codex:
        resource_id: petdef-codex
        i18n:
          en: {display_name: Codex}
          zh-CN: {display_name: Codex}
  gameplay:
    points:
      initial_balance: 100
    adoption:
      pool:
        - {pet_def: codex, weight: 100, rarity: common, adoption_cost: 10}
    rewards:
      default: {points_delta: 5, pet_exp_delta: 3}
```

Workflow aliases live under `workflows.collections.<collection>.<alias>`. Alias IDs are globally unique across Collections, while the client owns its fixed Collection navigation, ordering, icons, and Collection translations. RuntimeProfile supplies dynamic Workflow membership and alias-level `en` and `zh-CN` display text; it has no top-level locale or Collection presentation section.

The maps under `resources` bind environment aliases to canonical Admin resource IDs. Model and Voice aliases are independent environment variables, not Workflow members. Workflow specs and Workspace parameters store symbolic aliases, so each Workspace reload resolves the latest active binding. The same binary can therefore use production or debug RuntimeProfiles without rebuilding.

The normalized spec has an opaque deterministic revision. Catalog list/get responses include the RuntimeProfile name and revision. Pagination cursors are revision-bound. Each list, get, Workspace reload, and standalone Speech call obtains one current profile snapshot; a concurrent update affects the next operation.

## RegistrationToken

An administrator creates a `RegistrationToken` that names one RuntimeProfile. The raw token is returned only on creation and the Server stores its SHA-256 hash. `server.register` associates the connection with that RuntimeProfile name. Updating or switching the profile changes the environment used by later operations; it does not rewrite Workspace context or persisted aliases.

Public HTTP login may submit the same token through `X-Registration-Token`. Registration success and failure are logged without storing raw tokens in business data.

## Peer surface and ownership

- Workflow, Model, Voice, and Tool list/get return safe alias projections only. An AST Workflow projection includes its Workspace language-pair default so a client never infers behavior from the dynamic alias. Projections do not expose canonical IDs, providers, tenants, credentials, owners, or execution routing.
- Workflow list requires a Collection. Workflow get uses the globally unique alias. There is no `source=runtime|owned` selector.
- Workflow, Model, Credential, and Tool create/put/delete are not Peer RPC methods. Admin owns canonical resource management.
- Workspace create requires `collection` and `workflow_alias`; Workspace list requires `collection`. The Server stores Collection as an internal Workspace label and does not return generic labels through Peer RPC.
- A removed Workflow alias does not hide or delete its Workspace. List/get still return it, while reload/run fails with not found until the alias is restored.
- Pet instances remain Peer/domain state. Adoption and all reward values come from `gameplay`; Server config contains only operational settings.

Firmware remains an independent Admin resource and is not part of the RuntimeProfile projection. Credentials and ProviderTenants remain Server-only dependencies of canonical Model and Voice resources.
