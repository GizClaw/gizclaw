# RuntimeProfile and device registration

`RuntimeProfile` is the connection-scoped environment exposed to a device. Administrators create canonical Workflow, Model, Voice, Tool, and Path resources; a Peer cannot create those resources. A Peer may create Workspace state.

## Declarative structure

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  id: default
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
    memories:
      assistant-memory:
        layout_id: assistant-memory
        driver: flowcraft
        connection:
          type: flowcraft_redis8
          url: redis://redis:6379/0
    voices:
      cute-pet:
        resource_id: volc-tenant:volc-main:zh_male_naiqimengwa_mars_bigtts
        i18n:
          en: {display_name: Cute Pet}
          zh-CN: {display_name: 奶气萌宠}
```

`workflows` contains only `collections`. RuntimeProfile create and update validate every referenced canonical Workflow ID, its driver, and the Model, Voice, and Tool aliases used inside the Workflow. Friend and Friend Group Workspaces are always bound to the built-in `system-sfu` Workflow and are not selected through RuntimeProfile; see [services/social](/en/developing/gizclaw/services/social#sfu-workspace).

Optional Workflow aliases live under `workflows.collections.<collection>.<alias>`. Alias IDs are globally unique across Collections, while the client owns its fixed Collection navigation, ordering, icons, and Collection translations. RuntimeProfile supplies dynamic Workflow membership and alias-level `en` and `zh-CN` display text; it has no top-level locale or Collection presentation section.

The maps under `resources` bind environment aliases to canonical Admin resource IDs. Model aliases name semantic roles such as `chat`, `extraction`, `embedding`, `asr`, `realtime`, and `translation`; they do not contain provider or canonical Model names. Model and Voice aliases are independent environment variables, not Workflow members. Workflow specs and Workspace parameters store symbolic aliases, so each Workspace reload resolves the latest active binding. The same binary can therefore use production or debug RuntimeProfiles without rebuilding.

Every RuntimeProfile alias is 1-63 bytes of dot-separated lowercase kebab-case segments. Undotted names such as `asr` and `extract` identify shared capabilities; names such as `journey.model`, `journey.narrator`, and `story.journey-center-earth` provide independently bindable consumer slots. Each complete name remains one opaque key in a flat map. The Server preserves it exactly and performs no segment lookup, prefix matching, wildcard matching, or fallback from `journey.narrator` to `narrator`. Dotted and hyphenated forms such as `journey.narrator` and `journey-narrator` are distinct aliases. Empty segments, underscores, and leading or trailing hyphens within a segment are invalid.

`resources.memories` is the product-owned deployment binding for long-term Memory. Each alias selects one Admin `MemoryLayout`, one driver, and exactly one typed connection. The closed connection variants are managed local `flowcraft_bbh`, `flowcraft_object_store` (explicit directory), `flowcraft_postgresql` (DSN), `flowcraft_redis8` (Redis 8.4+ URL), `mem0` (endpoint, API key, Project ID), and `volc_mem0` (endpoint, API key, Memory Project ID). `flowcraft_bbh` stores data under the Server Workspace root and needs no external service, while `flowcraft_redis8` accepts `redis://` or certificate-verifying `rediss://`, with an optional `tls_ca_file` for an additional trusted CA. External connection values are stored directly in this Admin-only RuntimeProfile; they do not reference a Credential and are never projected through Peer APIs. Driver and connection type must match, and Flowcraft Layout model aliases must exist in the same RuntimeProfile.

The binding alias identifies the named physical source selected by a Workflow's scalar `memory` field. Within the same Workspace, driver, and physical binding, changing extraction policy, Graph Recall/Observe policy, prompts, or `top_k` does not create another canonical data namespace. Changing the driver or connection can select another source without migrating or deleting the old one. Deleting a Workspace purges only its data in the current binding; see [Memory Store](/en/developing/stores/memory#memorylayout-runtimeprofile-and-workflow).

`flowcraft_bbh` is no longer a supported connection. A persisted profile that still uses it is rejected on read or runtime resolution with the affected profile and binding names, but remains replaceable through `PUT` with `flowcraft_redis8` or `flowcraft_object_store`. GizClaw does not migrate, reinterpret, or delete the former managed local directory when the profile is rejected, replaced, or deleted; operators must retain or back up that directory and perform any data transfer explicitly before switching the binding.

## app_config

`app_config` is stored in the `app_config_json` column of the same RuntimeProfile row. Creation, updates, and every read path preserve the original string values. Omitted configuration is stored as JSON `null`, and an explicit empty map as `{}`; both represent no configuration and clear previous values on update. Initialization adds the column to existing tables that lack it while preserving other fields and versions. Administrators must resubmit configuration that was never stored.

`spec.app_config` is the optional opaque configuration downlink for the device itself. It puts device-owned product configuration into the RuntimeProfile the device already selects, so switching environments does not require a firmware rebuild. It is a key-value map: keys use exactly the RuntimeProfile alias syntax shared with every other binding (1-63 bytes of dot-separated lowercase kebab-case segments), and values are arbitrary strings.

The Server stores and returns each value verbatim: it never parses, trims, re-encodes, or checks whether a value is JSON. The encoding is the device's choice. The Server validates only the key syntax, a 4096-byte ceiling per value, and a 64-entry ceiling per profile, and rejects a write whose keys collide after normalization. Key syntax and the byte ceiling are both enforced during normalization: OpenAPI 3.0 has no `propertyNames` keyword and its `maxLength` counts characters, while Clients decode into static buffers sized in UTF-8 bytes, so normalization is stricter than the schema.

```yaml
spec:
  app_config:
    ui.theme: dark
    app.entrypoints: |
      {"home": "/tab/home", "settings": "/tab/settings"}
    feature.flags: beta-voice,beta-pet
```

app_config keys and binding aliases such as Workflow, Model, Voice, and Tool are independent namespaces and do not participate in global alias uniqueness: `chat` under `app_config` and `chat` under `resources.models` do not collide.

Clients read it through `server.app_config.list` and `server.app_config.get` and have no write method. Every device bound to one RuntimeProfile reads identical content; there is no per-Peer configuration. Any registered device holding that binding can read every key and value, so credentials, API keys, and other secrets must not be stored here; credentials stay in Credential and ProviderTenant, resolved on the Server and never projected.

app_config participates in spec normalization and revision computation, so changing configuration publishes a new revision and a Client can cache the revision and skip a refetch while it is unchanged.

The normalized spec has an opaque deterministic revision. Catalog list/get responses include the RuntimeProfile ID and revision. Pagination cursors are revision-bound. Each list, get, Workspace reload, and standalone Speech call obtains one current profile snapshot; a concurrent update affects the next operation.

RuntimeProfile create and update validate the complete dependency graph before publishing a revision. Snapshot reads, including Workspace reload, trust that persisted revision and do not traverse Workflow, Model, Voice, Tool, or Memory dependencies again. Each consumer resolves only the exact bindings it uses; an unavailable selected dependency fails in that consumer, while unrelated unavailable resources do not block the snapshot or an unaffected Workspace.

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

Firmware remains an independent Admin resource and is not part of the RuntimeProfile projection. A RegistrationToken may bind its Firmware ID independently of the RuntimeProfile, without binding a channel. Credentials and ProviderTenants remain Server-only dependencies of canonical Model and Voice resources.

RuntimeProfile uses the SQL `runtime_profiles`, `registration_tokens`, and `runtime_profile_owners` tables. Profile ID, configuration revision, token, referenced Profile/Firmware IDs, owner, and timestamps have separate columns; resource and Workflow configuration remain JSON. Startup also drops superseded `runtime_profiles` columns that an earlier release created as `NOT NULL`, so a database upgraded in place converges on the current schema instead of rejecting every write that omits them. A unique index enforces token uniqueness. Registration and owner-profile resolution use joins, and lists apply ID cursors and limits in SQL. Profile and token updates/deletes compare row version and creation identity. Owner binding writes use short transactions. External registration callbacks run outside SQL transactions; failure restores the previous binding only when the write identity still matches, protecting later updates. Registrations and snapshot publication for one owner remain serialized within the process while unrelated owners can proceed.
