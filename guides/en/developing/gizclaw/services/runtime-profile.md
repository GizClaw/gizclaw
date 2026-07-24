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
    system:
      friend_chatroom: chatroom
      group_chatroom: chatroom
      pet: pet-care
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
        - {pet_def: codex, weight: 100, rarity: common, adoption_cost: 10}
    pet:
      time:
        care_decay_per_hour: {health: 0.5, satiety: 1.3888888889, hygiene: 0.7, mood: 1}
        energy_recovery_per_hour: 10
        life_decay:
          max_loss_per_hour: 4
          exponent: 2
          contributing_weights: {health: 0.25, satiety: 0.25, hygiene: 0.25, mood: 0.25}
      experience:
        energy_per_pet_exp: 5
        leveling: {base_exp: 30, log_scale: 10}
      actions:
        feed: {energy_cost: 10, stat_delta: 10}
        bathe: {energy_cost: 10, stat_delta: 10}
        play: {energy_cost: 10, stat_delta: 10}
        heal: {energy_cost: 10, stat_delta: 10}
      games: {}
```

The three `workflows.system` values are canonical Admin-created Workflow IDs, not Collection aliases. Direct and group chats use `friend_chatroom` and `group_chatroom`; Pet adoption uses `pet`. RuntimeProfile create and update validate these IDs, their expected outer drivers, and the Model, Voice, and Tool aliases used inside those Workflows.

Optional Workflow aliases live under `workflows.collections.<collection>.<alias>`. Alias IDs are globally unique across Collections, while the client owns its fixed Collection navigation, ordering, icons, and Collection translations. RuntimeProfile supplies dynamic Workflow membership and alias-level `en` and `zh-CN` display text; it has no top-level locale or Collection presentation section.

The maps under `resources` bind environment aliases to canonical Admin resource IDs. Model aliases name semantic roles such as `chat`, `extraction`, `embedding`, `asr`, `realtime`, and `translation`; they do not contain provider or canonical Model names. Model and Voice aliases are independent environment variables, not Workflow members. Workflow specs and Workspace parameters store symbolic aliases, so each Workspace reload resolves the latest active binding. The same binary can therefore use production or debug RuntimeProfiles without rebuilding.

Each `gameplay.adoption.pool` entry references only a `pet_defs` alias. The localized PetDef name also comes from that RuntimeProfile binding rather than duplicated i18n in PetDef. PetDef stores only character/speaking style, PIXA metadata, and fixed behavior-to-animation bindings. Models, Voices, and Tools used by a Pet Workflow are symbolic aliases in the canonical Workflow spec and resolve through the system Workspace owner's RuntimeProfile.

`gameplay.pet` completely configures fixed-Pet time decay, passive energy recovery, leveling, and all four standard behaviors. `games` has no default. Each key must also exist in `resources.game_defs` and independently configures energy/points cost plus reward model, prompt, and maxima. Driving an unconfigured GameDef is a no-write no-op.

The normalized spec has an opaque deterministic revision. Catalog list/get responses include the RuntimeProfile name and revision. Pagination cursors are revision-bound. Each list, get, Workspace reload, and standalone Speech call obtains one current profile snapshot; a concurrent update affects the next operation.

## RegistrationToken

A `RegistrationToken` is an ordinary Admin-managed binding resource. Its required `spec.token` value selects one required RuntimeProfile name and, optionally, one Firmware release-line ID. Admin create, put, get, list, delete, apply, and show all use the same readable state. The Server persists that complete state and maintains a SHA-256 lookup index; changing the token atomically replaces the index, and identical apply input is unchanged.

RuntimeProfile and RegistrationToken have independent deployment ownership. Raids provides reusable
base resources plus the public `RuntimeProfile/default` and
`RegistrationToken/default-runtime` contract. Desktop consumes that pair for local Servers; its
deterministic UUID is a public enrollment identifier, not an Admin credential. Product platforms and
other deployments still own their RegistrationTokens and may independently install default or
product-specific profiles and bind explicit tokens to either.

`server.register` associates the connection with the RuntimeProfile, persists the owner's selected RuntimeProfile name and optional Firmware ID, and returns both selections. Owner-bound Workspaces resolve the current revision of that persisted profile name even while the owner is offline; a later successful registration replaces the owner's selection. Neither RegistrationToken nor Peer stores a Firmware channel: stable, beta, develop, or pending selection remains device-owned. Updating or switching the profile changes the environment used by later operations; it does not rewrite Workspace context or persisted aliases.

Public HTTP login may submit the same value through `X-Registration-Token`. Registration success and failure logs do not include the submitted token value.

## Peer surface and ownership

- Workflow, Model, Voice, and Tool list/get return safe alias projections only. An AST Workflow projection includes its Workspace language-pair default so a client never infers behavior from the dynamic alias. Projections do not expose canonical IDs, providers, tenants, credentials, owners, or execution routing.
- Workflow list requires a Collection. Workflow get uses the globally unique alias. There is no `source=runtime|owned` selector.
- Workflow, Model, Credential, and Tool create/put/delete are not Peer RPC methods. Admin owns canonical resource management.
- Workspace create requires `collection` and `workflow_alias`; Workspace list requires `collection`. The Server stores Collection as an internal Workspace label and does not return generic labels through Peer RPC.
- A removed Workflow alias does not hide or delete its Workspace. List/get still return it, while reload/run fails with not found until the alias is restored.
- Pet instances remain Peer/domain state. Adoption and all reward values come from `gameplay`; Server config contains only operational settings.

Firmware remains an independent Admin resource and is not part of the RuntimeProfile projection. A RegistrationToken may bind its release-line ID independently of the RuntimeProfile, without binding a channel. Credentials and ProviderTenants remain Server-only dependencies of canonical Model and Voice resources.
