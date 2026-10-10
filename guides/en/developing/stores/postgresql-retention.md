# PostgreSQL retention

pg_partman 5.5.0 manages PostgreSQL time partitions. Public LogStore and Memory Store interfaces do not depend on it; PostgreSQL adapters register and validate the tables. SQLite remains an independent backend.

## Deployment

`build/postgres/Dockerfile` supplies PostgreSQL 17, pgvector and pinned pg_partman. Build it from the repository root:

~~~sh
docker build -f build/postgres/Dockerfile -t gizclaw-postgres .
~~~

PostgreSQL startup must load `shared_preload_libraries=pg_partman_bgw` and set `pg_partman_bgw.dbname`, a privileged `pg_partman_bgw.role`, and `pg_partman_bgw.interval=3600`. Background maintenance prepares and removes partitions hourly, including when no business writes arrive. Time partitions use UTC.

Compose runs a repeatable initialization service from the same PostgreSQL image, with entrypoint `bootstrap-partman`. `PGHOST`, `PGUSER`, and `PGPASSWORD` select the administrative connection; `PG_PARTMAN_DATABASES` lists existing application databases separated by commas. Deployment injects administrative credentials; do not store them in configuration or images. The initializer waits for PostgreSQL health, and applications depend on it with `condition: service_completed_successfully`.

The initializer installs pgvector and pg_partman in each database as the administrator, and grants registration privileges to the database owner. Partitions inherit the parent owner and privileges; the initializer also repairs existing children so the application role can maintain partitions created by the administrative background worker. It runs against existing volumes too, independently of first-initialization docker-entrypoint-initdb.d scripts. Deployment still owns database and business-role creation; each database owner must match its application connection identity. See `tests/gizclaw-e2e/docker/compose.memory.yaml`, which isolates Server and Mem0 databases and roles.

## Retention boundaries

| Data | Partition key | Policy |
| --- | --- | --- |
| Workspace History | Write-time expires_at_unix_nano | Both History Store and asset ttl are 2160h; pg_partman adds zero retention |
| Peer/model usage | hour_unix_nano | pg_partman retention is 90 days |
| Mem0 PGVector | Physical expires_at | Fixed write time plus 90 days; pg_partman adds zero retention |

History and Mem0 keep independent tables. Reads hide expired records immediately. Physical reclamation drops complete UTC day partitions during hourly maintenance, so removal can lag a record deadline by one partition period plus the maintenance interval. Registration sets retention_keep_table=false; the default detach behavior does not delete data.

Server History maintenance and Mem0 ancillary maintenance run hourly too, pruning identities, reservations, SQLite journals/messages and orphaned entity indexes. PostgreSQL partition maintenance continues while applications are stopped; ancillary cleanup catches up on application startup. Binary History assets keep ObjectStore's separate 90-day TTL.

## Startup and existing data

Existing partitioned History and usage tables register automatically, preserving parent reads and current expiration semantics. pg_partman owns partition creation/removal. Child relation names are adjusted when required, without copying business records.

First startup converts ordinary Mem0 collections transactionally. The original collection name remains an updatable SDK view; UUIDs, payloads and vectors are copied unchanged. Row counts are checked before removing the old physical table; failure rolls back that collection. Initial conversion needs a table lock and index creation, so deployment requires a maintenance window. Repeated startup does not copy again.

Existing Mem0 records use valid creation time plus 90 days. Missing times start at conversion; future times are capped at conversion. Invalid dates abort startup and preserve the old table until source data is repaired. Cloud/Volc and local Qdrant do not use this PostgreSQL policy.
