# RuntimeProfile and device registration

`RuntimeProfile` is the connection-scoped environment exposed to a device. Administrators create canonical Workflow, Model, Voice, Tool, PetDef, GameDef, BadgeDef, and Path resources; a Peer cannot create those resources. A Peer may create Workspace state and adopt Pet instances.

## Declarative structure

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  id: default
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
    memories:
      pet-memory:
        layout_id: pet-memory
        driver: flowcraft
        connection:
          type: flowcraft_bbh
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

Every RuntimeProfile alias is 1-63 bytes of dot-separated lowercase kebab-case segments. Undotted names such as `asr` and `extract` identify shared capabilities; names such as `journey.model`, `journey.narrator`, and `story.journey-center-earth` provide independently bindable consumer slots. Each complete name remains one opaque key in a flat map. The Server preserves it exactly and performs no segment lookup, prefix matching, wildcard matching, or fallback from `journey.narrator` to `narrator`. Dotted and hyphenated forms such as `journey.narrator` and `journey-narrator` are distinct aliases. Empty segments, underscores, and leading or trailing hyphens within a segment are invalid.

`resources.memories` is the product-owned deployment binding for long-term Memory. Each alias selects one Admin `MemoryLayout`, one driver, and exactly one typed connection. The closed connection variants are `flowcraft_bbh` (managed under the Server Workspace root), `flowcraft_object_store` (explicit directory), `flowcraft_postgresql` (DSN), `mem0` (endpoint, API key, Project ID), and `volc_mem0` (endpoint, API key, Memory Project ID). The external values are stored directly in this Admin-only RuntimeProfile; they do not reference a Credential and are never projected through Peer APIs. Driver and connection type must match, and Flowcraft Layout model aliases must exist in the same RuntimeProfile.

The binding alias identifies the named physical source selected by a Workflow's scalar `memory` field. Within the same Workspace, driver, and physical binding, changing extraction policy, Graph Recall/Observe policy, prompts, or `top_k` does not create another canonical data namespace. Changing the driver or connection can select another source without migrating or deleting the old one.

Each `gameplay.adoption.pool` entry references only a `pet_defs` alias. The localized PetDef name also comes from that RuntimeProfile binding rather than duplicated i18n in PetDef. PetDef stores only character/speaking style, PIXA metadata, and fixed behavior-to-animation bindings. Models, Voices, and Tools used by a Pet Workflow are symbolic aliases in the canonical Workflow spec and resolve through the system Workspace owner's RuntimeProfile.

`gameplay.pet` completely configures fixed-Pet time decay, passive energy recovery, leveling, and all four standard behaviors. `games` has no default. Each key must also exist in `resources.game_defs` and independently configures energy/points cost plus reward model, prompt, and maxima. Driving an unconfigured GameDef is a no-write no-op.

`gameplay.workspace_reward` configures AI rewards for Workspace conversation
quality. When enabled, it must fully declare eligible Workspace kinds, debounce,
transcript bounds, the LLM evaluator, Points tiers, a Badge allowlist, and a
rolling budget. The evaluator model is an ordinary LLM alias in
`resources.models`. Each Badge alias must exist in `resources.badge_defs`, and
its BadgeDef must declare a non-empty `reward_prompt`. The `badges` map may be
empty for a Points-only policy. For example:

```yaml
resources:
  models:
    reward-evaluator:
      resource_id: reward-evaluator-model
  badge_defs:
    science:
      resource_id: badge-science
gameplay:
  points:
    initial_balance: 100
  workspace_reward:
    enabled: true
    workspace_kinds: [workflow, direct_chatroom, group_chatroom]
    debounce: {quiet_period: 2m, max_window_age: 15m}
    transcript: {max_entries: 100, max_text_bytes: 65536}
    evaluation:
      model: reward-evaluator
      points_prompt: Reward thoughtful conversation and demonstrated learning progress.
      score_min: 0
      score_max: 100
      qualifying_score: 80
    points:
      tiers:
      - {min_score: 80, delta: 5}
      - {min_score: 90, delta: 10}
    badges:
      science: {max_exp_per_window: 5}
    rolling_budget: {period: 24h, points_max: 50, badge_exp_max: 20}
```

`workspace_reward: {enabled: false}` is the canonical disabled form; an absent
field also grants no conversation rewards. The policy freezes when each
debounced window opens, so later RuntimeProfile or BadgeDef updates affect only
new windows. It does not register an Admin Tool, built-in Tool, or Toolkit.

The normalized spec has an opaque deterministic revision. Catalog list/get responses include the RuntimeProfile ID and revision. Pagination cursors are revision-bound. Each list, get, Workspace reload, and standalone Speech call obtains one current profile snapshot; a concurrent update affects the next operation.

## RegistrationToken

A `RegistrationToken` is an ordinary Admin-managed binding resource with caller-supplied `metadata.id`. Its required `spec.token` uses `runtime_profile_id` to select one canonical RuntimeProfile ID and may independently use `firmware_id` to bind one Firmware ID. Admin create, put, get, list, delete, apply, and show all use the same readable state. The Server persists that complete state and maintains a SHA-256 lookup index; changing the token atomically replaces the index, and applying the same ID and configuration is unchanged.

RuntimeProfile and RegistrationToken have independent deployment ownership. Raids provides reusable
base resources plus the public `RuntimeProfile/default` and
`RegistrationToken/default-runtime` contract. Desktop consumes that pair for local Servers; its
deterministic UUID is a public enrollment identifier, not an Admin credential. Product platforms and
other deployments still own their RegistrationTokens and may independently install default or
product-specific profiles and bind explicit tokens to either.

`server.register` associates the connection with the RuntimeProfile and persists canonical RuntimeProfile and optional Firmware IDs internally. The `runtime_profile_name` wire field carries the canonical RuntimeProfile ID verbatim because RuntimeProfile has no separate Peer name; this is the normal Peer-name projection rule, not a compatibility field. Registration returns no Firmware identity. The Server resolves Firmware only from the internal `firmware_id` binding, and `server.firmware.get` returns the selected channel configuration. Owner-bound Workspaces resolve the current revision from the persisted canonical RuntimeProfile ID even while the owner is offline; a later successful registration replaces the owner's selection. Neither RegistrationToken nor Peer stores a Firmware channel: stable, beta, or develop selection remains device-owned. Updating or switching the profile changes the environment used by later operations; it does not rewrite Workspace context or persisted internal bindings.

RegistrationToken is submitted only through `server.register` on a reliable Peer connection. Registration success and failure logs do not include the submitted token value; Public HTTP does not accept RegistrationToken.

## Peer surface and ownership

- Workflow, Model, Voice, and Tool list/get return safe scoped-name projections only. An AST Workflow projection includes its Workspace language-pair default so a client never infers behavior from a dynamic name. Projections do not expose canonical IDs, providers, tenants, credentials, owners, or execution routing.
- Workflow list requires a Collection. Workflow get uses the name projected by the current RuntimeProfile. There is no `source=runtime|owned` selector.
- Workflow, Model, Credential, and Tool create/put/delete are not Peer RPC methods. Admin owns canonical resource management.
- Workspace create requires `collection` and `workflow_name`; Workspace list requires `collection`. The Server stores Collection as an internal Workspace label and does not return generic labels through Peer RPC. The same typed create capability is used by OpenAI Conversation creation; Admin cannot create or apply a Workspace.
- A removed Workflow binding does not hide or delete its Workspace. List/get still return it, while reload/run fails with not found until the same Peer name is restored.
- Pet instances remain Peer/domain state. Adoption and all reward values come from `gameplay`; Server config contains only operational settings.

Firmware remains an independent Admin resource and is not part of the RuntimeProfile projection. A RegistrationToken may bind its Firmware ID independently of the RuntimeProfile, without binding a channel. Credentials and ProviderTenants remain Server-only dependencies of canonical Model and Voice resources.
