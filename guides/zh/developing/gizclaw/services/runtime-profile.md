# RuntimeProfile 与设备注册

`RuntimeProfile` 是设备连接能够看到的运行环境。Workflow、Model、Voice、Tool、PetDef、GameDef、BadgeDef 和 Path 等真实资源都由管理员创建；Peer 不能创建这些资源，只能创建 Workspace 状态和领养 Pet 实例。

## 声明式结构

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

Workflow alias 位于 `workflows.collections.<collection>.<alias>`。Alias ID 在所有 Collection 之间全局唯一；客户端拥有固定的 Collection 菜单、顺序、图标与 Collection 翻译。RuntimeProfile 只提供动态 Workflow 成员，以及 alias 自己的 `en`、`zh-CN` 显示文本，不包含顶层 locale 或 Collection 展示配置。

`resources` 下的 map 把环境 alias 绑定到管理员创建的真实资源 ID。Model alias 表示 `chat`、`extraction`、`embedding`、`asr`、`realtime`、`translation` 这类稳定用途，不包含 provider 或真实 Model 名。Model 和 Voice alias 是互相独立的环境变量，不属于 Workflow Collection。Workflow spec 和 Workspace 参数保存符号 alias；每次 Workspace reload 都从当前 RuntimeProfile 重新解析。因此同一个 App 或固件可以切换生产、调试 RuntimeProfile，而无需重新构建。

每个 `gameplay.adoption.pool` 条目必须同时引用一个 `pet_defs` alias 和一个 `voices` alias。PetDef 只保存宠物角色/说话风格、PIXA 元数据和固定行为到动画 clip 的绑定，不保存 Voice ID 或 Voice alias。领养时 Server 把池条目的 Voice alias 写入 system Workspace；以后重新绑定同名 alias 即可替换真实音色，无需修改 PetDef。

规范化后的 spec 有确定性的 opaque revision。Catalog list/get 响应携带 RuntimeProfile name 与 revision，分页 cursor 与 revision 绑定。每次 list、get、Workspace reload 和 standalone Speech 调用使用一个一致快照；并发更新从下一次操作开始生效。

## RegistrationToken

管理员创建 `RegistrationToken` 时必须指定一个 RuntimeProfile name，也可以独立指定一个 Firmware release-line ID。Raw token 只在创建时返回，Server 仅保存 SHA-256 hash。`server.register` 把连接关联到 RuntimeProfile，把可选 Firmware ID 持久化到 Peer，并在响应中返回这两个选择。RegistrationToken 和 Peer 都不保存 Firmware channel；stable、beta、develop 或 pending 由设备自行选择。更新或切换 RuntimeProfile 只改变后续操作使用的环境，不重写 Workspace context 或已经保存的 alias。

公开 HTTP login 也可以通过 `X-Registration-Token` 提交同一个 token。注册成功或失败会写日志，但业务数据不保存 raw token。

## Peer surface 与 ownership

- Workflow、Model、Voice 和 Tool list/get 只返回安全 alias projection。AST Workflow projection 会携带 Workspace 默认语言对，客户端不再从动态 alias 推断行为；projection 不暴露真实 ID、provider、tenant、credential、owner 或 executor routing。
- Workflow list 必须传 Collection；Workflow get 只传全局唯一 alias；不存在 `source=runtime|owned`。
- Peer RPC 不提供 Workflow、Model、Credential 和 Tool create/put/delete；真实资源统一由 Admin 管理。
- Workspace create 必须传 `collection` 与 `workflow_alias`，Workspace list 必须传 `collection`。Server 把 Collection 保存为内部 Workspace label，但 Peer RPC 不返回通用 labels。
- Workflow alias 删除后，不隐藏也不删除 Workspace。list/get 仍返回 Workspace，reload/run 在 alias 恢复前返回 not found。
- Pet 实例仍是 Peer/领域状态；领养与所有 reward 数值都来自 `gameplay`，Server config 只保存运行参数。

Firmware 仍是独立 Admin 资源，不进入 RuntimeProfile projection。RegistrationToken 可以独立绑定 Firmware release-line ID，但不绑定 channel。Credential 与 ProviderTenant 只是真实 Model、Voice 在 Server 侧使用的依赖，不会暴露给设备。
