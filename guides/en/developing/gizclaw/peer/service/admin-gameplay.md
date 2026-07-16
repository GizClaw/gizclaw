# Admin HTTP · Gameplay

`Implementation file: peer_service_serve_admin_gameplay.go`

Implements Admin endpoints to query pet, badge, points, points transaction, game result, and reward grant by Peer, and to delete a Pet through its owning Gameplay lifecycle.

Gameplay resources and state belong to `services/gameplay`.

## Core structure and main function

| Function group | Function |
| --- | --- |
| `ListPeerPets` / `GetPeerPet` / `DeletePeerPet` | Query or delete a Peer pet. Deletion delegates all Workspace and ACL cleanup to Gameplay. |
| `ListPeerBadges` / `GetPeerBadge` | Query Peer badge. |
| `GetPeerPoints` | Query Peer points account. |
| `ListPeerPointsTransactions` / `GetPeerPointsTransaction` | Query points transactions. |
| `ListPeerGameResults` / `GetPeerGameResult` | Query game results. |
| `ListPeerRewardGrants` / `GetPeerRewardGrant` | Query reward grants. |
| `gameplayNotConfiguredResponse` | Generate Gameplay unconfigured response. |
