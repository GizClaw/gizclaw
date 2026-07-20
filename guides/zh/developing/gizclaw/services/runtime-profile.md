# RuntimeProfile 与设备注册

`RuntimeProfile` 定义设备连接可用的服务器资源和 Gameplay 配置。它与资源的 owner 机制互补：设备可以访问 RuntimeProfile 允许的资源，也可以访问自己拥有的资源；查询结果先列 RuntimeProfile 资源，再列 owner 资源。

## Declarative structure

```yaml
apiVersion: gizclaw.admin/v1alpha1
kind: RuntimeProfile
metadata:
  name: h106-tragon
spec:
  resources:
    workflows:
      chat: general-chat
    models:
      primary: model-default
    voices:
      assistant: voice-default
    tools:
      weather: weather-v2
    pet_defs:
      tragon: petdef-tragon
    game_defs:
      dinodive: game-dinodive
    badge_defs:
      dinodive-master: badge-dinodive-master
  gameplay:
    points:
      initial_balance: 100
    pet_pool:
      - pet_def: tragon
        weight: 100
        rarity: common
        adoption_cost: 10
    drive:
      game_rewards:
        dinodive:
          points_delta: 20
          badge_exp_delta:
            dinodive-master: 100
```

`resources` 中每个 map 都是 profile-local alias 到真实资源名的映射，映射的 value 组成 allow list。Gameplay 配置只在当前 RuntimeProfile 内使用 alias，例如 `pet_def: tragon` 对应 `petdef-tragon`。Workflow 是唯一把 alias namespace 暴露给公开资源 RPC 的类型：`server.workflow.list/get` 使用 `source=runtime` 时，RPC `id` 就是 alias；其他资源 RPC 仍使用真实资源名。

同一个真实资源可以绑定多个 alias。使用真实资源名的资源列表会对 value 去重；runtime Workflow 列表会保留每个 alias，因为 alias 是不同的客户端 ID。被引用的真实资源已经删除或不存在时，加载方忽略该项，不阻止 RuntimeProfile 保存、加载或删除。RuntimeProfile 不包含 icon、显示名称或 i18n；产品客户端按 alias 自己映射展示信息。

## RegistrationToken

`RegistrationToken` 由管理员预先创建，只关联一个 RuntimeProfile。原始 token 只在创建响应中返回一次，服务端只保存 SHA-256 hash。Token 可重复用于多个注册，直到被删除；没有 enable/disable 状态，也不保存使用记录或 public-key binding。RegistrationToken 名称额外接受 `app:<bundle-id>` 形式，不改变通用自定义资源 ID 的语法。

Raw token 通过客户端自己的安全注册通道交付。客户端连接后调用 `server.register`；服务端校验 token，并把 RuntimeProfile 快照保存到当前 connection，响应只包含 `runtime_profile_name`。修改 RuntimeProfile 不会改变已经建立的连接，客户端 reconnect 后重新注册才会取得新配置。

公开 HTTP 客户端在 `POST /login` 时通过可选的 `X-Registration-Token` header 提交同一个 token。得到的 bearer session 保存对应的 RuntimeProfile 快照，因此 `/openai/v1` 不依赖并行存在的 Peer RPC connection，也能解析相同的 RuntimeProfile Model 和 Voice。

注册成功和失败都写入 system log。日志包含 Peer public key、连接来源、RegistrationToken 名称和 RuntimeProfile；业务数据库不保存 token 使用历史。

## 访问规则

| 来源 | list / get / use | put / delete |
| --- | --- | --- |
| RuntimeProfile allow list | 允许，不检查 owner | 不允许 |
| 当前 Peer 是 owner | 允许 | 允许 |
| Friend、FriendGroup 或 Pet 的 system Workspace | 按领域关系允许 | 按领域规则处理 |
| 其他资源 | 不可见、不可用 | 不允许 |

未注册设备仍可以调用公开 RPC；它只是没有 RuntimeProfile 资源。设备通过公开 CRUD 创建的 Workspace、Workflow、Model、Credential 和 Tool 自动记录当前 Peer 为 owner。Runtime Workflow 只读，owned Workflow 支持公开 CRUD。

调用 Model 或 Voice 时，服务端在内部解析其配置的 ProviderTenant 和底层 Credential。RuntimeProfile 允许的 Model 或 Voice 可以使用对应的服务端 Credential，但不会让 Credential 出现在 credential list/get 中，也不会授予修改权限。RuntimeProfile 之外由 owner 创建的 Model 只能使用同一个 Peer 拥有的 Credential，不能通过 ProviderTenant 选择无关的服务端 Credential。

Firmware 保持独立的 Admin 管理资源，不由 RegistrationToken 选择，不进入 connection 注册状态，也不通过 Peer Firmware RPC 投影。删除 RuntimeProfile 或 RegistrationToken 不级联删除其他资源；已经建立的 connection 继续使用自己的 RuntimeProfile 快照，直到断线。
