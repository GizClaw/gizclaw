# services/gameplay

`pkgs/gizclaw/services/gameplay` 拥有 Gameplay catalog、玩家状态、奖励行为和数字资产。Gameplay 配置属于连接的 RuntimeProfile，不再有独立 GameRuleset 资源。

## Ownership

Gameplay 拥有 PetDef、BadgeDef、GameDef、Pet、points account、transaction、reward grant、badge progression 和 game result。RuntimeProfile 的 `pet_defs`、`game_defs` 和 `badge_defs` map 提供 profile-local alias；`gameplay.pet_pool` 与 reward 配置通过 alias 引用这些定义。

领养 Pet 时，服务从当前 connection 的 RuntimeProfile snapshot 解析规则，并把 RuntimeProfile 名写入 Pet 和相关状态。Pet 创建的 system Workspace 使用内置 `pet-care` Workflow；`pet-care` 不需要出现在 RuntimeProfile 的 `workflows` map 中。

被 alias 映射的定义不存在时会被忽略。没有有效 PetDef 的 profile 不能领养 Pet；未在当前 profile 中允许的 GameDef 不能提交 game result。删除定义或 RuntimeProfile 不级联删除已有 Gameplay 历史。

Gameplay 使用 Workspace owner 和 Pet 领域关系，不创建额外 role 或 policy binding。Pet 删除会先清理 system Workspace，成功后删除 Pet row，并保留 points、badge、result、transaction 和 reward grant 历史。
