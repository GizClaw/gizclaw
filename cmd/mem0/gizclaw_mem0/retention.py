"""Ninety-day PGVector retention with pg_partman and unchanged Mem0 SDK SQL.

The SDK's logical collection is an updatable view. Its physical vector rows
are partitioned by expiration; identity rows enforce UUID uniqueness across
partitions. Existing collections are converted transactionally at startup.
"""

import hashlib
import logging
import re
from datetime import datetime, timedelta, timezone

import psycopg
from psycopg import sql
from mem0.memory.main import _build_session_scope

RETENTION = timedelta(days=90)
INTERVAL = 3600
_identifier = re.compile(r"^[A-Za-z_][A-Za-z0-9_]{0,62}$")
_log = logging.getLogger(__name__)


def _name(collection, suffix):
    digest = hashlib.sha256(collection.encode()).hexdigest()[:8]
    return f"{collection[:38]}_{digest}_{suffix}"


class Retention:
    def __init__(self, memory):
        self.memory = memory
        self.dsn = memory.config.vector_store.config.connection_string
        self.stores = [memory.vector_store, memory.entity_store]
        for store in self.stores:
            self.prepare(store)
        self.maintain()

    def prepare(self, store):
        collection = store.collection_name
        if not _identifier.fullmatch(collection):
            raise RuntimeError("Mem0 retention requires a portable collection name")
        records, keys = _name(collection, "records"), _name(collection, "keys")
        trigger = _name(collection, "identity")
        legacy = _name(collection, "legacy")
        with psycopg.connect(self.dsn, connect_timeout=5) as db:
            db.execute("SET LOCAL TIME ZONE 'UTC'")
            db.execute("SELECT pg_advisory_xact_lock(hashtext('gizclaw.mem0.retention'), hashtext(%s))", (collection,))
            if not db.execute("SELECT EXISTS(SELECT 1 FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_partman' AND n.nspname='partman')").fetchone()[0]:
                raise RuntimeError("Mem0 PGVector retention requires pg_partman in schema partman")
            db.execute("CREATE EXTENSION IF NOT EXISTS vector")
            kind = db.execute("SELECT relkind FROM pg_class WHERE oid=to_regclass(%s)", (collection,)).fetchone()
            migrate = kind is not None and kind[0] == "r"
            if kind is not None and kind[0] not in ("r", "v"):
                raise RuntimeError("Unsupported Mem0 collection relation")
            if kind is not None and kind[0] == "v" and not db.execute("SELECT to_regclass(%s)", (records,)).fetchone()[0]:
                raise RuntimeError("Mem0 collection view is not owned by the retention adapter")
            if migrate:
                db.execute(sql.SQL("LOCK TABLE {} IN ACCESS EXCLUSIVE MODE").format(sql.Identifier(collection)))
                db.execute(sql.SQL("ALTER TABLE {} RENAME TO {}").format(sql.Identifier(collection), sql.Identifier(legacy)))
            db.execute(sql.SQL("CREATE TABLE IF NOT EXISTS {} (id UUID PRIMARY KEY, expires_at timestamptz NOT NULL, routing JSONB NOT NULL)").format(sql.Identifier(keys)))
            db.execute(sql.SQL("CREATE INDEX IF NOT EXISTS {} ON {} (expires_at)").format(sql.Identifier(_name(collection, "key_expiry")), sql.Identifier(keys)))
            db.execute(sql.SQL("""
                CREATE TABLE IF NOT EXISTS {} (
                    id UUID NOT NULL, vector vector({}), payload JSONB,
                    expires_at timestamptz NOT NULL DEFAULT (statement_timestamp() + interval '90 days'),
                    PRIMARY KEY(id, expires_at)
                ) PARTITION BY RANGE(expires_at)
            """).format(sql.Identifier(records), sql.Literal(store.embedding_model_dims)))
            db.execute(sql.SQL("""
                CREATE OR REPLACE FUNCTION {}() RETURNS trigger LANGUAGE plpgsql AS $$
                BEGIN
                    IF TG_OP = 'INSERT' THEN
                        DELETE FROM {} WHERE id=NEW.id AND expires_at <= statement_timestamp();
                        INSERT INTO {} VALUES (NEW.id, NEW.expires_at,
                            jsonb_strip_nulls(jsonb_build_object('user_id',NEW.payload->'user_id','agent_id',NEW.payload->'agent_id','run_id',NEW.payload->'run_id')));
                        RETURN NEW;
                    END IF;
                    -- Routing survives an SDK delete until scope erasure or expiry.
                    RETURN OLD;
                END $$
            """).format(sql.Identifier(trigger), sql.Identifier(keys), sql.Identifier(keys)))
            if not db.execute("SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=to_regclass(%s) AND tgname=%s)", (records, trigger)).fetchone()[0]:
                db.execute(sql.SQL("CREATE TRIGGER {} BEFORE INSERT OR DELETE ON {} FOR EACH ROW EXECUTE FUNCTION {}()").format(
                    sql.Identifier(trigger), sql.Identifier(records), sql.Identifier(trigger)))
            schema = db.execute("SELECT current_schema()").fetchone()[0]
            parent = schema + "." + records
            registered = db.execute("SELECT EXISTS(SELECT 1 FROM partman.part_config WHERE parent_table=%s)", (parent,)).fetchone()[0]
            if not registered:
                db.execute("SELECT partman.create_parent(%s,'expires_at','1 day',p_premake:=92,p_default_table:=false,p_jobmon:=false)", (parent,))
            db.execute("""
                UPDATE partman.part_config SET retention='0 seconds', retention_keep_table=false,
                    infinite_time_partitions=true WHERE parent_table=%s
            """, (parent,))
            if migrate:
                days = db.execute(sql.SQL("""
                    SELECT DISTINCT date_trunc('day', LEAST(COALESCE((payload->>'created_at')::timestamptz, statement_timestamp()),statement_timestamp()) + interval '90 days')
                    FROM {} ORDER BY 1
                """).format(sql.Identifier(legacy))).fetchall()
                for (day,) in days:
                    db.execute("SELECT partman.create_partition_time(%s,ARRAY[%s]::timestamptz[])", (parent, day))
                db.execute(sql.SQL("""
                    INSERT INTO {}(id,vector,payload,expires_at)
                    SELECT id,vector,payload,LEAST(COALESCE((payload->>'created_at')::timestamptz,statement_timestamp()),statement_timestamp()) + interval '90 days' FROM {}
                """).format(sql.Identifier(records), sql.Identifier(legacy)))
                old_count = db.execute(sql.SQL("SELECT count(*) FROM {}").format(sql.Identifier(legacy))).fetchone()[0]
                new_count = db.execute(sql.SQL("SELECT count(*) FROM {}").format(sql.Identifier(records))).fetchone()[0]
                if old_count != new_count:
                    raise RuntimeError("Mem0 migration row count mismatch")
                db.execute(sql.SQL("DROP TABLE {}").format(sql.Identifier(legacy)))
            db.execute(sql.SQL("""
                CREATE OR REPLACE VIEW {} AS SELECT id,vector,payload FROM {}
                WHERE expires_at > statement_timestamp() WITH LOCAL CHECK OPTION
            """).format(sql.Identifier(collection), sql.Identifier(records)))
            if store.use_diskann:
                raise RuntimeError("Mem0 retention supports HNSW or exact vector search")
            if store.use_hnsw:
                db.execute(sql.SQL("CREATE INDEX IF NOT EXISTS {} ON {} USING hnsw(vector vector_cosine_ops)").format(
                    sql.Identifier(_name(collection, "hnsw")), sql.Identifier(records)))
            db.execute(sql.SQL("CREATE INDEX IF NOT EXISTS {} ON {} USING gin(to_tsvector('simple',payload->>'text_lemmatized'))").format(
                sql.Identifier(_name(collection, "keyword")), sql.Identifier(records)))
            db.execute("SELECT partman.run_maintenance(%s,p_analyze:=false,p_jobmon:=false)", (parent,))
        # Do not run the SDK's CREATE TABLE/INDEX statements against the view.
        store._collection_ensured = True

    def maintain(self):
        now = datetime.now(timezone.utc)
        expired_ids = []
        with psycopg.connect(self.dsn, connect_timeout=5) as db:
            db.execute("SET LOCAL TIME ZONE 'UTC'")
            db.execute("SET LOCAL statement_timeout='30s'")
            db.execute("SET LOCAL lock_timeout='5s'")
            if not db.execute("SELECT pg_try_advisory_xact_lock(hashtext('gizclaw.mem0.retention'), hashtext(%s))", (self.memory.collection_name,)).fetchone()[0]:
                return
            schema = db.execute("SELECT current_schema()").fetchone()[0]
            for store in self.stores:
                parent = schema + "." + _name(store.collection_name, "records")
                db.execute("SELECT partman.run_maintenance(%s,p_analyze:=false,p_jobmon:=false)", (parent,))
                keys = _name(store.collection_name, "keys")
                rows = db.execute(sql.SQL("DELETE FROM {} WHERE expires_at <= %s RETURNING id").format(sql.Identifier(keys)), (now,)).fetchall()
                expired_ids.extend(str(row[0]) for row in rows)
            # Remove entities with no live fact. Updating/removing these rows
            # uses the same logical view and UUID ownership trigger as SDK deletes.
            db.execute(sql.SQL("""
                DELETE FROM {} AS entity WHERE jsonb_typeof(entity.payload->'linked_memory_ids')='array'
                AND NOT EXISTS (
                    SELECT 1 FROM jsonb_array_elements_text(entity.payload->'linked_memory_ids') AS link(id)
                    JOIN {} AS identity ON identity.id=CASE
                        WHEN pg_input_is_valid(link.id,'uuid') THEN link.id::uuid END
                        AND identity.expires_at > statement_timestamp()
                    JOIN {} AS fact ON fact.id=identity.id AND fact.expires_at=identity.expires_at
                )
            """).format(sql.Identifier(self.stores[1].collection_name),
                          sql.Identifier(_name(self.stores[0].collection_name, "keys")),
                          sql.Identifier(_name(self.stores[0].collection_name, "records"))))
            # Receipts cannot retain deleted fact text and only enforce retry
            # identity. Add a deadline to make abandoned reservations bounded.
            receipt = db.execute("SELECT to_regclass('gizclaw_direct_observations')").fetchone()[0]
            if receipt:
                db.execute("ALTER TABLE gizclaw_direct_observations ADD COLUMN IF NOT EXISTS expires_at timestamptz NOT NULL DEFAULT (statement_timestamp()+interval '90 days')")
                # Keep completed reservations until expiry even if an individual
                # fact was deleted, so a stale retry cannot resurrect that fact.
                db.execute("DELETE FROM gizclaw_direct_observations WHERE collection=%s AND expires_at <= %s", (self.memory.collection_name, now))
        history = self.memory.db
        with history._lock, history.connection:
            db = history.connection
            db.execute("CREATE INDEX IF NOT EXISTS history_retention ON history(julianday(created_at))")
            db.execute("CREATE INDEX IF NOT EXISTS messages_retention ON messages(julianday(created_at))")
            cutoff = (now - RETENTION).isoformat()
            for table in ("history", "messages"):
                db.execute(f"DELETE FROM {table} WHERE julianday(created_at) <= julianday(?)", (cutoff,))
            for offset in range(0, len(expired_ids), 500):
                ids = expired_ids[offset:offset+500]
                db.execute("DELETE FROM history WHERE memory_id IN (" + ",".join("?" for _ in ids) + ")", ids)
            db.commit()

    def purge(self, routing):
        """Erase hidden expired rows, journals and compound-route caches."""
        clause = sql.SQL(" AND ").join(sql.SQL("payload->>{}={}").format(sql.Literal(field), sql.Placeholder()) for field in routing)
        identity_clause = sql.SQL(" AND ").join(sql.SQL("routing->>{}={}").format(sql.Literal(field), sql.Placeholder()) for field in routing)
        values = tuple(routing.values())
        ids = []
        with psycopg.connect(self.dsn, connect_timeout=5) as db:
            db.execute("SET LOCAL statement_timeout='30s'")
            db.execute("SET LOCAL lock_timeout='5s'")
            for store in self.stores:
                keys = _name(store.collection_name, "keys")
                ids.extend(str(row[0]) for row in db.execute(sql.SQL("SELECT id FROM {} WHERE {} FOR UPDATE").format(sql.Identifier(keys), identity_clause), values))
                db.execute(sql.SQL("DELETE FROM {} WHERE {}").format(sql.Identifier(_name(store.collection_name, "records")), clause), values)
            # Keep routing until the SQLite write commits, so a failed write
            # rolls back PG and a subsequent retry can still find journal IDs.
            history = self.memory.db
            with history._lock, history.connection:
                for offset in range(0, len(ids), 500):
                    batch = ids[offset:offset+500]
                    history.connection.execute("DELETE FROM history WHERE memory_id IN (" + ",".join("?" for _ in batch) + ")", batch)
                conditions = []
                scope_values = []
                for field, value in routing.items():
                    conditions.append("instr('&'||session_scope||'&', ?) > 0")
                    scope_values.append("&" + _build_session_scope({field: value}) + "&")
                history.connection.execute("DELETE FROM messages WHERE " + " AND ".join(conditions), scope_values)
            for store in self.stores:
                db.execute(sql.SQL("DELETE FROM {} WHERE {}").format(sql.Identifier(_name(store.collection_name, "keys")), identity_clause), values)

    async def run(self):
        import anyio
        while True:
            await anyio.sleep(INTERVAL)
            try:
                await anyio.to_thread.run_sync(self.maintain)
            except Exception:
                _log.error("Mem0 retention maintenance failed; retrying next interval")
