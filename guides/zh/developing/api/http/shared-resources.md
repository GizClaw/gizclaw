# Shared 与 Resources

`api/http/shared/` 与 `api/http/resources/` 表达不同所有权。Shared 是跨 surface 或跨领域复用的 contract；Resources 是 Admin 声明式资源及其专属数据。

## shared

`shared/` 只保存具有至少两个实际 consumers，或明确属于跨领域 contract 的数据结构、enum 和 value object。`shared.json` 聚合这些定义，并通过 `#/components/schemas` 提供给多个 HTTP surfaces 和 codegen。

适合放在 `shared/` 的内容：

- request/response 之间共享的稳定 DTO；
- 有独立语义的 enum、spec 或 nested value；
- 多个 HTTP surface 需要使用的 error 和 pagination 类型。

不适合放在这里的内容：单个 Resource 专属 Spec、只有一个父 schema 的小 value object、database row、service dependency、runtime lock、内部 cache state，以及只由单个 handler 使用的临时结构。

## Shared 列表

`shared/` 包含以下按 owner 组织的 schema 文件。

### 跨 HTTP surface

| 文件 | 包含的 schema | 实际消费者 |
| --- | --- | --- |
| `shared/error.json` | `ErrorPayload`、`ErrorResponse` | Admin、Peer、Desktop HTTP |
| `shared/device.json` | `DeviceInfo`、`HardwareInfo`、`PeerIMEI`、`PeerLabel` | Admin Peer view、Peer self/registration |
| `shared/runtime.json` | `Runtime` 及其共同 runtime value | Admin Peer runtime、Peer self runtime |

这是确定的跨 surface shared 集合。`PeerRegistrationStatus`、`PeerStatus` 和 `ServerInfo` 目前只属于 Public API，应直接定义在 `public.json`，不能因为它们包含 “Peer” 就放入 shared。

### Admin API 与多个 Resources 共用

| 文件 | 包含的 schema family | 复用边界 |
| --- | --- | --- |
| `shared/acl.json` | Permission、Policy、Resource、Subject、Role/View 公共 value | ACL Admin endpoints 与多个 ACL Resources |
| `shared/configuration.json` | `Configuration`、firmware/agent selection 等配置 value | Peer/registration model 与 PeerConfig Resource |
| `shared/gameplay.json` | Gameplay metadata、Pet、Badge、Points、Game Result、共同规则 value | Gameplay Admin endpoints 与 Game/Pet/Badge Resources |
| `shared/firmware.json` | Firmware、slot、artifact 与 selection 公共 value | Firmware Admin endpoints 与 Firmware Resource |
| `shared/credential.json` | Credential body 与可复用 credential value | Credential Admin endpoint 与 Credential Resource |
| `shared/model.json` | Model kind、capabilities、provider、source 与 provider data | Model Admin endpoint 与 Model Resource |
| `shared/voice.json` | Voice provider、source 与 provider data | Voice Admin endpoint 与 Voice Resource |
| `shared/tool.json` | Tool executor、trigger、source 与 JSON schema value | Tool APIs、Workflow/Toolkit 与 Tool Resource |
| `shared/workflow.json` | Workflow document、metadata、driver 与各 workflow variant | Workflow Admin API、Workspace parameters 与 Workflow Resource |
| `shared/workspace.json` | Workspace parameters、input mode 与共同 workspace value | Workspace Admin API、Workflow runtime 与 Workspace Resource |
| `shared/provider-tenants.json` | Provider tenant 共同 enum/value | Model、Voice 与各 Provider Tenant Resources |

这里保留的是被 Admin response/request 与 Resource spec 共同引用的 value，并不包含 Resource envelope。每个 Resource 专属的 `*Spec` 仍放在对应 `resources/*.json`；如果某个 value 最终只剩一个 owner，也应继续内联，而不是因为出现在本表就永久保留。

### 明确不属于 Shared

| 类型 | 所属位置 |
| --- | --- |
| `PeerRegistrationStatus`、`PeerStatus`、`ServerInfo` | `public.json` |
| `ResourceAPIVersion`、`ResourceKind`、`ResourceMetadata` | `resources/base.json` |
| `ApplyAction`、`ApplyResult`、Resource union | `resources/base.json` 或 `resources.json` |
| `ModelSpec`、`VoiceSpec`、`FirmwareSpec` 等单一 Resource Spec | 对应 `resources/<resource>.json` |
| OpenAI-compatible request/response models | `openai-compat/v1/service.json` |
| Desktop-only context、view 与 session models | `apps/wails` Desktop contract |

`shared.json` 只导出上面 Shared 列表中的 schema。它不得为了让 `apitypes` 一次生成所有 symbol，而重新导出整个 Resource graph。

## resources

`resources/` 描述 Admin 声明式资源。它们服务于 `admin apply`、`admin show` 和 resource manager，不被 Peer HTTP 或 Desktop surface 使用。

一个 resource schema 应表达：

- resource kind 与稳定 identity；
- 用户可以声明的 spec；
- apply/show 需要保留的 metadata；
- 与其他 resource 的显式引用。

### 核心数据与 Display

Resource 的数据首先按语义分为两类：

- 核心数据描述 Resource 是什么以及它与什么关联，包括稳定 identity、kind、分类、引用、ownership、运行配置和持久化语义。这些字段参与业务判断、查询、关联和执行，不能放进 `display`。
- `display` 只描述如何把该 Resource 展示给用户，例如本地化名称、subtitle、description、icon 和 cover。删除或替换 `display` 不得改变 Resource 的关联关系或运行行为。

需要通用展示 metadata 的 Resource 自己拥有可选的 `display` 字段，并在对应 `resources/<kind>.json` 中定义自己的强类型 Display schema。例如 Workflow 定义 `WorkflowDisplay`，Workspace 定义 `WorkspaceDisplay`。即使两个 Resource 当前需要相同的字段，也不能因此建立公共 `ResourceDisplay`、`ResourceDisplayData` 或通用 catalog schema。

如果某个 Resource 只需要本地化 catalog，不需要额外的展示 metadata，可以直接拥有语义更准确的 `i18n` 字段。例如 PetDef 使用自己的 `PetDefI18n`：`i18n.default_locale` 指定默认语言，`i18n.en`、`i18n.zh-CN` 等 locale key 直接保存对应 catalog，不增加 `catalogs` 或 `display` 中间层。`display` 与 `i18n` 都是 Resource-owned contract，不进入 Shared。

Display 的共同命名是一项结构约定，不代表公共领域模型。不同 Resource 可以独立增加符合自身产品语义的展示字段；修改一个 Resource 的 Display 不应迫使无关 Resource 同步修改或重新生成 API。

判断字段归属时使用下面的规则：

| 问题 | 是 | 否 |
| --- | --- | --- |
| 字段是否影响 identity、关联、过滤、授权、执行或持久化语义？ | 放入 Resource 的核心字段或 `spec` | 继续判断 |
| 字段是否只用于面向人的名称、说明或视觉呈现？ | 放入该 Resource 自己的 `display` | 不应为它创建 Display 字段 |
| 相同字段是否由多个 Resource 使用？ | 各 Resource 仍拥有自己的 Display 定义 | 不以“看起来相同”为理由放入 Shared |

`category`、关联 ID、workflow reference、provider kind 等机器可读字段属于核心数据。`display_name`、本地化说明、icon 和 cover 属于 Display。客户端在 `display` 缺失时可以回退到稳定 ID，但 Server 不应把 fallback 文本持久化为核心数据。

“视觉内容”不自动等于 `display`。如果 asset、clip、animation graph 或 action-to-clip mapping 被设备、runtime 或领域逻辑直接消费，它就是 Resource 的核心内容或关联数据。例如 PetDef 的 PIXA、canvas、clips、visual refs 和 `visual_clip_id` 属于 PetDef spec；`display` 只保存管理界面或用户阅读所需的展示 metadata 与本地化文本。

只被一个 Resource 使用的 Spec 应与 Resource 定义在同一文件。例如只有 Model Resource 使用的 `ModelSpec` 应位于 `resources/model.json`，不再单独建立 `shared/model_spec.json`。

运行时连接、stream、临时状态和 provider client 不能塞进 resource spec。Resource 表达期望状态，领域 service 负责校验并实现该状态。

## 复用关系

```mermaid
flowchart LR
    Shared["shared/*.json"] --> SharedIndex["shared.json"]
    Shared --> Resource["resources/*.json"]
    Resource --> ResourceIndex["resources.json"]
    SharedIndex --> Admin["admin.json"]
    ResourceIndex --> Admin
    SharedIndex --> Public["public.json"]
```

依赖必须是 `shared ← resources ← admin`，以及 `shared ← public/openai-compatible`。`shared.json` 不能反向聚合 `resources/`。

新增字段时应优先修改其真正拥有者：真正共享的 value 修改 `shared/`，声明式资源和专属 Spec 修改 `resources/`，只属于某个 endpoint 的输入则留在该 surface。不要复制一份名字相近但逐渐漂移的 schema。

## 稳定性边界

Schema name、property name、required 集合、enum value、discriminator 和 OpenAPI operation ID 都会影响生成 API。重命名或改变 optional/nullable 语义属于 caller-facing contract 变化，必须与所有生成语言和调用点一起审查。
