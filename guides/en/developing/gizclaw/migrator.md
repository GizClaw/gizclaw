# migrator

The root `Migrator` runs existing Peer and Credential domain migrations. RuntimeProfile, RegistrationToken, and owner indexes use KV and need no SQL migration. The access-model replacement does not migrate old ACL data; deployments remove old server data and rebuild it.

```text
ServeContext
└── NewMigrator(config)
    ├── open Peer / Credential stores
    └── gizclaw.Migrator.Migrate
        ├── Peers.Migration
        └── Credentials.Migration
```

Gameplay SQL migration still runs from Gameplay runtime initialization and is not part of the root `Migrator`. Each domain owns its schema and data transforms; the root only orders calls and joins errors.
