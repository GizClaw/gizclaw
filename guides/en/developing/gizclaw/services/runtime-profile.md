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
      chat:
        resource_id: doubao-seed-2-0-lite
        i18n:
          en: {display_name: Chat}
          zh-CN: {display_name: 对话}
      extraction:
        resource_id: deepseek-v4-flash
        i18n:
          en: {display_name: Extraction}
          zh-CN: {display_name: 信息提取}
      embedding:
        resource_id: qwen3.7-text-embedding
        i18n:
          en: {display_name: Embedding}
          zh-CN: {display_name: 文本向量}
      asr:
        resource_id: volc-bigasr-sauc
        i18n:
          en: {display_name: Speech Recognition}
          zh-CN: {display_name: 语音识别}
    voices:
      cute-pet:
        resource_id: volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts
        i18n:
          en: {display_name: Cute Pet}
          zh-CN: {display_name: 奶气萌宠}
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
        - {pet_def: codex, voice: cute-pet, weight: 100, rarity: common, adoption_cost: 10}
    rewards:
      default: {points_delta: 5, pet_exp_delta: 3}
```

Workflow aliases live under `workflows.collections.<collection>.<alias>`. Alias IDs are globally unique across Collections, while the client owns its fixed Collection navigation, ordering, icons, and Collection translations. RuntimeProfile supplies dynamic Workflow membership and alias-level `en` and `zh-CN` display text; it has no top-level locale or Collection presentation section.

The maps under `resources` bind environment aliases to canonical Admin resource IDs. Model aliases name semantic roles such as `chat`, `extraction`, `embedding`, `asr`, `realtime`, and `translation`; they do not contain provider or canonical Model names. Model and Voice aliases are independent environment variables, not Workflow members. Workflow specs and Workspace parameters store symbolic aliases, so each Workspace reload resolves the latest active binding. The same binary can therefore use production or debug RuntimeProfiles without rebuilding.

Every `gameplay.adoption.pool` entry references both a `pet_defs` alias and a `voices` alias. PetDef stores the Pet character/speaking style, PIXA metadata, and fixed behavior-to-animation bindings, but no Voice ID or Voice alias. On adoption, the Server writes the pool entry's Voice alias into the system Workspace; rebinding that alias later changes the canonical voice without editing PetDef.

The normalized spec has an opaque deterministic revision. Catalog list/get responses include the RuntimeProfile name and revision. Pagination cursors are revision-bound. Each list, get, Workspace reload, and standalone Speech call obtains one current profile snapshot; a concurrent update affects the next operation.

## RegistrationToken

An administrator creates a `RegistrationToken` with one required RuntimeProfile name and, optionally, one Firmware release-line ID. The raw token is returned only on creation and the Server stores its SHA-256 hash. `server.register` associates the connection with the RuntimeProfile, persists the optional Firmware ID on the Peer, and returns both selections. Neither RegistrationToken nor Peer stores a Firmware channel: stable, beta, develop, or pending selection remains device-owned. Updating or switching the profile changes the environment used by later operations; it does not rewrite Workspace context or persisted aliases.

Public HTTP login may submit the same token through `X-Registration-Token`. Registration success and failure are logged without storing raw tokens in business data.

## Peer surface and ownership

- Workflow, Model, Voice, and Tool list/get return safe alias projections only. An AST Workflow projection includes its Workspace language-pair default so a client never infers behavior from the dynamic alias. Projections do not expose canonical IDs, providers, tenants, credentials, owners, or execution routing.
- Workflow list requires a Collection. Workflow get uses the globally unique alias. There is no `source=runtime|owned` selector.
- Workflow, Model, Credential, and Tool create/put/delete are not Peer RPC methods. Admin owns canonical resource management.
- Workspace create requires `collection` and `workflow_alias`; Workspace list requires `collection`. The Server stores Collection as an internal Workspace label and does not return generic labels through Peer RPC.
- A removed Workflow alias does not hide or delete its Workspace. List/get still return it, while reload/run fails with not found until the alias is restored.
- Pet instances remain Peer/domain state. Adoption and all reward values come from `gameplay`; Server config contains only operational settings.

Firmware remains an independent Admin resource and is not part of the RuntimeProfile projection. A RegistrationToken may bind its release-line ID independently of the RuntimeProfile, without binding a channel. Credentials and ProviderTenants remain Server-only dependencies of canonical Model and Voice resources.
