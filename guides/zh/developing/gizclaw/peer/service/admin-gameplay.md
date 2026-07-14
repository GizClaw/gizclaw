# Admin HTTP · Gameplay

`实现文件：peer_service_serve_admin_gameplay.go`

实现按 Peer 查询 pet、badge、points、points transaction、game result 和 reward grant 的 Admin 只读 endpoints。

Gameplay 资源和状态属于 `services/gameplay`。

## 核心结构与主函数

| 函数组 | 作用 |
| --- | --- |
| `ListPeerPets` / `GetPeerPet` | 查询 Peer pet。 |
| `ListPeerBadges` / `GetPeerBadge` | 查询 Peer badge。 |
| `GetPeerPoints` | 查询 Peer points account。 |
| `ListPeerPointsTransactions` / `GetPeerPointsTransaction` | 查询 points transactions。 |
| `ListPeerGameResults` / `GetPeerGameResult` | 查询 game results。 |
| `ListPeerRewardGrants` / `GetPeerRewardGrant` | 查询 reward grants。 |
| `gameplayNotConfiguredResponse` | 生成 Gameplay 未配置响应。 |
