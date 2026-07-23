# Admin HTTP · Gameplay

`Implementation file: peer_service_serve_admin_gameplay.go`

Implements Admin endpoints to query pet, badge, points, points transaction, game result, and reward grant by Peer, and to delete a Pet through its owning Gameplay lifecycle.

Gameplay resources and state belong to `services/gameplay`.

## Core structure and main function

| Function group | Function |
| --- | --- |
| `ListPeerPets` / `GetPeerPet` / `DeletePeerPet` | Query or request deletion for a Peer Pet. Deletion records or reuses its PendingDeletion through Gameplay while retaining the Pet and system Workspace binding. |
| `ListPeerBadges` / `GetPeerBadge` | Query Peer badge. |
| `GetPeerPoints` | Query Peer points account. |
| `ListPeerPointsTransactions` / `GetPeerPointsTransaction` | Query points transactions. |
| `ListPeerGameResults` / `GetPeerGameResult` | Query game results. |
| `ListPeerRewardGrants` / `GetPeerRewardGrant` | Query reward grants. |
| `gameplayNotConfiguredResponse` | Generate Gameplay unconfigured response. |
