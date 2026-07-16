# Admin HTTP · Gameplay

`Implementation file: peer_service_serve_admin_gameplay.go`

Implement Admin read-only endpoints to query pet, badge, points, points transaction, game result and reward grant by Peer.

Gameplay resources and state belong to `services/gameplay`.

## Core structure and main function

| Function group | Function |
| --- | --- |
| `ListPeerPets` / `GetPeerPet` | Query Peer pet. |
| `ListPeerBadges` / `GetPeerBadge` | Query Peer badge. |
| `GetPeerPoints` | Query Peer points account. |
| `ListPeerPointsTransactions` / `GetPeerPointsTransaction` | Query points transactions. |
| `ListPeerGameResults` / `GetPeerGameResult` | Query game results. |
| `ListPeerRewardGrants` / `GetPeerRewardGrant` | Query reward grants. |
| `gameplayNotConfiguredResponse` | Generate Gameplay unconfigured response. |
