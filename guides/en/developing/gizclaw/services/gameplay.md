# services/gameplay

`pkgs/gizclaw/services/gameplay` owns the Gameplay catalog, player state, rewards, and digital assets. Gameplay configuration now belongs to a connection's RuntimeProfile; there is no separate GameRuleset resource.

## Ownership

Gameplay owns PetDef, BadgeDef, GameDef, Pet, points accounts, transactions, reward grants, badge progression, and game results. RuntimeProfile `pet_defs`, `game_defs`, and `badge_defs` maps provide profile-local aliases used by `gameplay.pet_pool` and reward configuration.

Pet adoption resolves rules from the current connection's RuntimeProfile snapshot and records the RuntimeProfile name on the Pet and related state. The Pet system Workspace uses the built-in `pet-care` Workflow; `pet-care` does not need to appear in the RuntimeProfile `workflows` map.

A definition missing behind an alias is skipped. A profile with no valid PetDef cannot adopt a Pet, and a GameDef not allowed by the current profile cannot submit a game result. Deleting a definition or RuntimeProfile does not cascade into existing Gameplay history.

Gameplay uses Workspace ownership and the Pet domain relationship. It does not create extra roles or policy bindings. Pet deletion removes its system Workspace before the Pet row and preserves points, badge, result, transaction, and reward-grant history.
