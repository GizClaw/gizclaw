# RuntimeProfile 与设备注册

`RuntimeProfile` 是设备连接能够看到的运行环境。Workflow、Model、Voice、Tool 和 Path 等真实资源都由管理员创建；Peer 不能创建这些资源，只能创建 Workspace 状态。

## 声明式结构

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

`workflows` 只包含 `collections`。RuntimeProfile 创建或更新时会验证每个引用的真实 Workflow ID、其 driver，以及 Workflow 内部使用的 Model、Voice、Tool alias。Friend 与 Friend Group 的 Workspace 固定绑定内置 `system-sfu` Workflow，不经 RuntimeProfile 选择，见 [services/social](/zh/developing/gizclaw/services/social#sfu-workspace)。

可选 Workflow alias 位于 `workflows.collections.<collection>.<alias>`。Alias ID 在所有 Collection 之间全局唯一；客户端拥有固定的 Collection 菜单、顺序、图标与 Collection 翻译。RuntimeProfile 只提供动态 Workflow 成员，以及 alias 自己的 `en`、`zh-CN` 显示文本，不包含顶层 locale 或 Collection 展示配置。

`resources` 下的 map 把环境 alias 绑定到管理员创建的真实资源 ID。Model alias 表示 `chat`、`extraction`、`embedding`、`asr`、`realtime`、`translation` 这类稳定用途，不包含 provider 或真实 Model 名。Model 和 Voice alias 是互相独立的环境变量，不属于 Workflow Collection。Workflow spec 和 Workspace 参数保存符号 alias；每次 Workspace reload 都从当前 RuntimeProfile 重新解析。因此同一个 App 或固件可以切换生产、调试 RuntimeProfile，而无需重新构建。

每个 RuntimeProfile alias 都是总长 1–63 字节、由 `.` 分隔的 lowercase kebab-case segment。`asr`、`extract` 等无点名称表示共享能力；`journey.model`、`journey.narrator`、`story.journey-center-earth` 等名称表示可独立绑定的 consumer 槽位。完整名称始终是平面 map 中的一个 opaque key；Server 原样保留，不按 segment 查找，不支持 prefix、wildcard，也不会从 `journey.narrator` fallback 到 `narrator`。`journey.narrator` 与 `journey-narrator` 是两个不同 alias。空 segment、下划线以及 segment 内的首尾连字符均不合法。

`resources.memories` 是产品拥有的长期 Memory 部署 binding。每个 alias 选择一个 Admin `MemoryLayout`、一个 driver 和唯一的 typed connection。封闭的 connection variant 包括：托管本地 `flowcraft_bbh`、显式目录 `flowcraft_object_store`、DSN 形式的 `flowcraft_postgresql`、Redis 8.4+ URL 形式的 `flowcraft_redis8`、带 endpoint/API key/Project ID 的 `mem0`，以及带 endpoint/API key/Memory Project ID 的 `volc_mem0`。`flowcraft_bbh` 将数据写入 Server Workspace root，不依赖外部服务；`flowcraft_redis8` 接受 `redis://` 或启用证书校验的 `rediss://`，后者可选 `tls_ca_file`。外部连接值直接保存在这个仅 Admin 可读的 RuntimeProfile 中，不引用 Credential，也不会通过 Peer API projection 暴露。Driver 必须与 connection type 匹配；Flowcraft Layout 使用的 model alias 必须存在于同一 RuntimeProfile。

这个 binding alias 表示 Workflow 标量 `memory` 字段选择的 named physical source。在相同 Workspace、driver 与 physical binding 下，修改 extraction policy、Graph Recall/Observe policy、prompt 或 `top_k` 不会创建新的 canonical data namespace；修改 driver 或 connection 可以切换到另一个数据源，但不会自动迁移或删除旧数据。

`flowcraft_bbh` 不再是受支持的 connection。仍使用它的已持久化 profile 会在读取或 runtime 解析时被拒绝，错误会指出具体 profile 与 binding；管理员仍可通过 `PUT` 将其显式替换为 `flowcraft_redis8` 或 `flowcraft_object_store`。Profile 被拒绝、替换或删除时，GizClaw 都不会迁移、重新解释或删除旧的 managed local directory；operator 必须先保留或备份该目录，并在切换 binding 前显式完成所需的数据转移。

## app_config

`spec.app_config` 是可选的不透明设备配置下发通道，把设备自己的产品配置放进它已经使用的 RuntimeProfile，而不需要重新构建固件。它是一个 key-value map：key 使用与其他 RuntimeProfile alias 完全相同的语法（1–63 字节、`.` 分隔的 lowercase kebab-case segment），value 是任意 string。

Server 原样存储并返回每个 value：不解析、不 trim、不做编码转换，也不校验它是不是 JSON。value 用什么格式由设备自己决定。Server 只校验 key 语法、单个 value 不超过 4096 字节、条目不超过 64 个；normalize 后出现重复 key 时拒绝整次写入。key 语法与 value 的字节上限都在 Server 归一化时校验：OpenAPI 3.0 没有 `propertyNames`，`maxLength` 也只能表达字符数，而设备侧按 UTF-8 字节静态分配，因此归一化比 schema 更严格。

```yaml
spec:
  app_config:
    ui.theme: dark
    app.entrypoints: |
      {"home": "/tab/home", "settings": "/tab/settings"}
    feature.flags: beta-voice,beta-pet
```

app_config key 与 Workflow、Model、Voice、Tool 等 binding alias 属于互相独立的命名空间，不参与全局 alias 唯一性检查：`app_config` 里的 `chat` 与 `resources.models` 里的 `chat` 互不冲突。

设备通过 `server.app_config.list` 与 `server.app_config.get` 只读访问，没有写入方法；同一个 RuntimeProfile 下的所有设备读到相同内容，不存在 per-Peer 配置。任何持有该绑定的已注册设备都能读到全部 key 与 value，因此这里不能存放 credential、API key 或任何 secret；凭证仍由 Credential 与 ProviderTenant 在 Server 侧解析，不进入 projection。

app_config 参与 spec 归一化和 revision 计算，因此改配置就会产生新的 revision，设备可以缓存 revision 并在未变化时跳过重新拉取。

规范化后的 spec 有确定性的 opaque revision。Catalog list/get 响应携带 RuntimeProfile ID 与 revision，分页 cursor 与 revision 绑定。每次 list、get、Workspace reload 和 standalone Speech 调用使用一个一致快照；并发更新从下一次操作开始生效。

RuntimeProfile 在创建和更新时校验完整依赖图，再发布新的 revision。Workspace reload 等快照读取信任已经持久化的 revision，不会再次遍历 Workflow、Model、Voice、Tool 或 Memory 依赖。每个 consumer 只解析自己实际使用的 binding：选中的依赖不可用时由该 consumer 返回错误，而无关资源不可用不会阻塞快照或不受影响的 Workspace。

## RegistrationToken

`RegistrationToken` 是普通的 Admin binding 资源。它自己的 `metadata.id` 由调用方提供；必填的 `spec.token` 通过 `runtime_profile_id` 绑定一个 RuntimeProfile canonical ID，也可以通过 `firmware_id` 独立绑定一个 Firmware ID。Admin create、put、get、list、delete、apply 和 show 使用同一份可读状态。Server 持久化完整状态并维护 SHA-256 lookup index；修改 token 时会原子替换 index，重复 apply 相同 ID 和配置则返回 unchanged。

RuntimeProfile 与 RegistrationToken 的部署 ownership 相互独立。Raids 提供可复用基础资源及
公开的 `RuntimeProfile/default`、`RegistrationToken/default-runtime` 契约。Desktop 为本地
Server 消费这对资源；其中确定性 UUID 是公开注册标识，不是 Admin 凭证。产品平台和其他
部署仍拥有自己的 RegistrationToken，可独立安装 default 或产品专用 profile，并把显式
token 绑定到任意一个。

`server.register` 把连接关联到 RuntimeProfile，内部持久化 canonical RuntimeProfile ID 与可选 Firmware ID。`runtime_profile_name` wire 字段原样携带 canonical RuntimeProfile ID，因为 RuntimeProfile 没有独立的 Peer name；这是正常的 Peer name 投影规则，不是兼容字段。Registration 不返回 Firmware identity；Server 只通过内部 `firmware_id` binding 解析 Firmware，`server.firmware.get` 仅返回所选 channel 的配置。Owner-bound Workspace 即使在 owner 离线时，也会通过持久化的 canonical RuntimeProfile ID 解析当前 revision；owner 后续成功注册可替换该选择。RegistrationToken 和 Peer 都不保存 Firmware channel；stable、beta 或 develop 由设备自行选择。更新或切换 RuntimeProfile 只改变后续操作使用的环境，不重写 Workspace context 或已经保存的内部 binding。

RegistrationToken 只通过可靠 Peer connection 上的 `server.register` 提交。注册成功或失败日志不包含提交的 token 值；Public HTTP 不接受 RegistrationToken。

## Peer surface 与 ownership

- Workflow、Model、Voice 和 Tool list/get 只返回安全的 scoped-name projection。AST Workflow projection 会携带 Workspace 默认语言对，客户端不再从动态 name 推断行为；projection 不暴露真实 ID、provider、tenant、credential、owner 或 executor routing。
- Workflow list 必须传 Collection；Workflow get 只传当前 RuntimeProfile 投影出的 name；不存在 `source=runtime|owned`。
- Peer RPC 不提供 Workflow、Model、Credential 和 Tool create/put/delete；真实资源统一由 Admin 管理。
- Workspace create 必须传 `collection` 与 `workflow_name`，Workspace list 必须传 `collection`。Server 把 Collection 保存为内部 Workspace label，但 Peer RPC 不返回通用 labels。同一个 typed create capability 也供 OpenAI Conversation 创建使用；Admin 不能 create 或 apply Workspace。
- Workflow binding 删除后，不隐藏也不删除 Workspace。list/get 仍返回 Workspace，reload/run 在相同 Peer name 恢复前返回 not found。

Firmware 仍是独立 Admin 资源，不进入 RuntimeProfile projection。RegistrationToken 可以独立绑定 Firmware ID，但不绑定 channel。Credential 与 ProviderTenant 只是真实 Model、Voice 在 Server 侧使用的依赖，不会暴露给设备。

RuntimeProfile 使用 SQL `runtime_profiles`、`registration_tokens` 和 `runtime_profile_owners` 表。Profile ID、配置 revision、token、关联 Profile/Firmware ID、owner 和时间分别保存为列；资源与 Workflow 配置保留 JSON。启动时还会删除旧版本在 `runtime_profiles` 上建立的、已废弃的 `NOT NULL` 列，使原地升级的数据库收敛到当前 schema，而不是让每次省略这些列的写入失败。token 使用唯一索引，注册解析和 Owner Profile 解析通过关联查询完成；列表将 ID 游标和数量限制下推 SQL。Profile 与 token 更新、删除比较行版本和创建标识。Owner 绑定写入使用短事务；外部注册回调在 SQL 事务外执行，失败时按本次写入标识恢复旧绑定，避免覆盖后续更新。本进程的同 Owner 注册与快照发布保持串行，无关 Owner 可以继续注册。
