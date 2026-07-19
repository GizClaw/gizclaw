# migrator

根 `Migrator` 负责已有 Peer 与 Credential 数据的领域迁移。RuntimeProfile、RegistrationToken 和 owner index 都使用 KV，不需要 SQL migration；本次访问模型切换也不迁移旧 ACL 数据，部署时删除旧服务器数据并重建。

```text
ServeContext
└── NewMigrator(config)
    ├── open Peer / Credential stores
    └── gizclaw.Migrator.Migrate
        ├── Peers.Migration
        └── Credentials.Migration
```

Gameplay SQL migration 仍由 Gameplay runtime 初始化路径执行，不属于根 `Migrator`。具体 schema 或数据转换由对应领域 service 拥有；根 `Migrator` 只确定执行顺序并聚合错误。
