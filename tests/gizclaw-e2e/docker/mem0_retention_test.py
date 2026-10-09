"""Real pg_partman migration and idle retention; no model API calls."""

import asyncio
import os
import tempfile
import time
import unittest
import uuid
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from unittest import mock

import anyio
import psycopg
from psycopg import sql
from mem0 import Memory

from gizclaw_mem0 import retention
from gizclaw_mem0 import direct, server
from fastapi import HTTPException


class RetentionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.dsn = os.environ["MEM0_POSTGRES_DSN"]
        cls.directory = tempfile.TemporaryDirectory()
        cls.collection = "retention_" + uuid.uuid4().hex
        cls.memory = Memory.from_config({
            "version": "v1.1", "history_db_path": cls.directory.name + "/history.db",
            "llm": {"provider": "openai", "config": {"model": "fixture", "api_key": "fixture"}},
            "embedder": {"provider": "openai", "config": {"model": "fixture", "api_key": "fixture", "embedding_dims": 2}},
            "vector_store": {"provider": "pgvector", "config": {
                "connection_string": cls.dsn, "collection_name": cls.collection,
                "embedding_model_dims": 2, "hnsw": False, "minconn": 1, "maxconn": 4,
            }},
        })
        cls.memory.vector_store._ensure_collection()
        cls.kept = str(uuid.uuid4())
        cls.old = str(uuid.uuid4())
        now = datetime.now(timezone.utc)
        cls.memory.vector_store.insert(
            vectors=[[1.0, 0.0], [0.0, 1.0]], ids=[cls.kept, cls.old],
            payloads=[
                {"data": "retained original fact", "user_id": "a", "created_at": (now-timedelta(days=4)).isoformat()},
                {"data": "expired original fact", "user_id": "a", "created_at": (now-timedelta(days=100)).isoformat()},
            ],
        )
        cls.cleanup = retention.Retention(cls.memory)

    @classmethod
    def tearDownClass(cls):
        with psycopg.connect(cls.dsn) as db:
            for store in cls.cleanup.stores:
                records = retention._name(store.collection_name, "records")
                parent = "public." + records
                template = db.execute("SELECT template_table FROM partman.part_config WHERE parent_table=%s", (parent,)).fetchone()
                db.execute("DELETE FROM partman.part_config WHERE parent_table=%s", (parent,))
                db.execute(sql.SQL("DROP VIEW IF EXISTS {}").format(sql.Identifier(store.collection_name)))
                for suffix in ("records", "keys"):
                    db.execute(sql.SQL("DROP TABLE IF EXISTS {} CASCADE").format(sql.Identifier(retention._name(store.collection_name, suffix))))
                db.execute(sql.SQL("DROP FUNCTION IF EXISTS {}()").format(sql.Identifier(retention._name(store.collection_name, "identity"))))
                if template:
                    db.execute(sql.SQL("DROP TABLE IF EXISTS {}").format(sql.Identifier(*template[0].split("."))))
        cls.memory.vector_store.connection_pool.close()
        cls.memory.entity_store.connection_pool.close()
        cls.memory.llm.client.close()
        cls.memory.embedding_model.client.close()
        cls.memory.close()
        cls.directory.cleanup()

    def test_migration_keeps_ids_vectors_and_restart_is_idempotent(self):
        original = self.memory.get(self.kept)
        self.assertEqual(original["memory"], "retained original fact")
        self.assertIsNone(self.memory.get(self.old))
        matches = self.memory.vector_store.search(query="fixture", vectors=[1.0, 0.0], filters={"user_id": "a"}, top_k=10)
        self.assertEqual([match.id for match in matches], [self.kept])
        retention.Retention(self.memory)
        self.assertEqual(self.memory.get(self.kept), original)
        with psycopg.connect(self.dsn) as db:
            record = db.execute(sql.SQL("SELECT expires_at FROM {} WHERE id=%s").format(
                sql.Identifier(retention._name(self.collection, "records"))), (self.kept,)).fetchone()
            self.assertLess(record[0], datetime.now(timezone.utc)+timedelta(days=87))

    def test_sdk_writes_scope_reads_updates_and_deletes(self):
        with mock.patch.object(self.memory.embedding_model, "embed", return_value=[1.0, 0.0]):
            result = self.memory.add([{"role": "user", "content": "new isolated fact"}], user_id="b", infer=False)
        fact_id = result["results"][0]["id"]
        self.assertEqual(self.memory.get(fact_id)["memory"], "new isolated fact")
        self.assertEqual([fact["id"] for fact in self.memory.get_all(filters={"user_id": "b"})["results"]], [fact_id])
        self.assertNotIn(fact_id, [fact["id"] for fact in self.memory.get_all(filters={"user_id": "a"})["results"]])
        with mock.patch.object(self.memory.embedding_model, "embed", return_value=[0.0, 1.0]):
            self.memory.update(fact_id, "updated fact")
        self.assertEqual(self.memory.get(fact_id)["memory"], "updated fact")
        self.memory.delete(fact_id)
        self.assertIsNone(self.memory.get(fact_id))
        with psycopg.connect(self.dsn) as db:
            self.assertEqual(db.execute(sql.SQL("SELECT count(*) FROM {} WHERE id=%s").format(
                sql.Identifier(retention._name(self.collection, "records"))), (fact_id,)).fetchone()[0], 0)

    def test_parallel_native_reads_keep_the_prepared_collection(self):
        with ThreadPoolExecutor(max_workers=8) as pool:
            results = list(pool.map(lambda _: self.memory.get(self.kept), range(16)))
        self.assertTrue(all(result["memory"] == "retained original fact" for result in results))

    def test_entity_cleanup_preserves_live_links_and_removes_orphans(self):
        kept, orphan, malformed = [str(uuid.uuid4()) for _ in range(3)]
        store = self.memory.entity_store
        store.insert(vectors=[[1.0,0.0]]*3, ids=[kept,orphan,malformed], payloads=[
            {"linked_memory_ids":[self.kept]},
            {"linked_memory_ids":[str(uuid.uuid4())]},
            {"linked_memory_ids":["invalid-uuid"]},
        ])
        self.cleanup.maintain()
        self.assertIsNotNone(store.get(kept))
        self.assertIsNone(store.get(orphan))
        self.assertIsNone(store.get(malformed))
        store.delete(kept)

    def test_maintenance_keeps_deleted_fact_retry_reservation(self):
        routing = {"user_id":"deleted-retry"}
        request = server.MemoryCreate(**routing, infer=False, observation_id="deleted-turn",
                                      observation_digest="a"*64,
                                      messages=[{"role":"user","content":"must stay deleted"}])
        with mock.patch.object(self.memory.embedding_model, "embed", return_value=[1.0,0.0]):
            result = direct.observe(self.memory, request, routing)
            self.memory.delete(result["results"][0]["id"])
            self.cleanup.maintain()
            with self.assertRaises(HTTPException) as error:
                direct.observe(self.memory, request, routing)
            self.assertEqual(error.exception.status_code, 409)
        with direct.purge(self.memory, routing):
            self.cleanup.purge(routing)

    def test_invalid_legacy_date_rolls_back_without_losing_the_original_table(self):
        from mem0.vector_stores.pgvector import PGVector
        collection = "invalid_" + uuid.uuid4().hex
        store = PGVector(collection_name=collection, embedding_model_dims=2,
                         connection_string=self.dsn, dbname=None, user=None,
                         password=None, host=None, port=None, diskann=False,
                         hnsw=False, minconn=1, maxconn=2)
        self.addCleanup(store.connection_pool.close)
        self.addCleanup(store.delete_col)
        fact_id = str(uuid.uuid4())
        store.insert(vectors=[[1.0,0.0]], ids=[fact_id],
                     payloads=[{"data":"must survive","created_at":"invalid-date"}])
        with self.assertRaises(psycopg.errors.InvalidDatetimeFormat):
            self.cleanup.prepare(store)
        self.assertEqual(store.get(fact_id).payload["data"], "must survive")
        with psycopg.connect(self.dsn) as db:
            self.assertEqual(db.execute("SELECT relkind FROM pg_class WHERE oid=to_regclass(%s)", (collection,)).fetchone()[0], "r")

    def test_idle_worker_cleans_expired_vectors_history_and_cache(self):
        old_id = str(uuid.uuid4())
        old_time = datetime.now(timezone.utc)-timedelta(days=100)
        expiry = datetime.now(timezone.utc)-timedelta(days=2)
        records = retention._name(self.collection, "records")
        with psycopg.connect(self.dsn) as db:
            db.execute("SELECT partman.create_partition_time(%s,ARRAY[date_trunc('day',%s::timestamptz)]::timestamptz[])", ("public."+records, expiry))
            db.execute(sql.SQL("INSERT INTO {}(id,vector,payload,expires_at) VALUES (%s,%s,%s,%s)").format(sql.Identifier(records)),
                       (old_id, [1.0, 0.0], '{"data":"expired"}', expiry))
        self.memory.db.add_history(old_id, None, "expired text", "ADD", created_at=old_time.isoformat())
        self.memory.db.save_messages([{"role": "user", "content": "expired context"}], "old-session")
        with self.memory.db._lock:
            self.memory.db.connection.execute("UPDATE messages SET created_at=? WHERE session_scope='old-session'", (old_time.isoformat(),))
            self.memory.db.connection.commit()

        async def run():
            with mock.patch.object(retention, "INTERVAL", 0.05):
                async with anyio.create_task_group() as tasks:
                    tasks.start_soon(self.cleanup.run)
                    deadline = time.monotonic()+30
                    while time.monotonic() < deadline:
                        await anyio.sleep(0.1)
                        with self.memory.db._lock:
                            cached = self.memory.db.connection.execute("SELECT count(*) FROM messages WHERE session_scope='old-session'").fetchone()[0]
                        if cached == 0:
                            break
                    else:
                        self.fail("idle retention worker did not run")
                    tasks.cancel_scope.cancel()
        asyncio.run(run())
        self.assertEqual(self.memory.db.get_history(old_id), [])
        with psycopg.connect(self.dsn) as db:
            self.assertEqual(db.execute(sql.SQL("SELECT count(*) FROM {} WHERE id=%s").format(sql.Identifier(records)), (old_id,)).fetchone()[0], 0)

    def test_scope_erasure_preserves_other_scopes_and_cleans_deleted_audits(self):
        with mock.patch.object(self.memory.embedding_model, "embed", return_value=[1.0, 0.0]):
            result = self.memory.add([{"role":"user","content":"scope deletion fact"}], user_id="erased", infer=False)
        fact_id = result["results"][0]["id"]
        # This row is expired but its UTC day partition is not wholly expired.
        # SDK list/delete hides it; scope erasure must still remove it physically.
        hidden_id = str(uuid.uuid4())
        expired = datetime.now(timezone.utc)-timedelta(minutes=1)
        records = retention._name(self.collection, "records")
        with psycopg.connect(self.dsn) as db:
            db.execute("SELECT partman.create_partition_time(%s,ARRAY[date_trunc('day',%s::timestamptz)]::timestamptz[])", ("public."+records, expired))
            db.execute(sql.SQL("INSERT INTO {}(id,vector,payload,expires_at) VALUES (%s,%s,%s,%s)").format(sql.Identifier(records)),
                       (hidden_id, [1.0,0.0], '{"data":"hidden","user_id":"erased"}', expired))
        self.memory.db.save_messages([{"role":"user","content":"erased context"}], "agent_id=x&user_id=erased")
        self.memory.db.save_messages([{"role":"user","content":"keep context"}], "user_id=erased-other")
        self.memory.delete_all(user_id="erased")
        self.cleanup.purge({"user_id":"erased"})
        with psycopg.connect(self.dsn) as db:
            self.assertEqual(db.execute(sql.SQL("SELECT count(*) FROM {} WHERE id=%s").format(sql.Identifier(records)), (hidden_id,)).fetchone()[0], 0)
        self.assertEqual(self.memory.db.get_history(fact_id), [])
        self.assertEqual(self.memory.db.get_last_messages("agent_id=x&user_id=erased"), [])
        self.assertEqual(len(self.memory.db.get_last_messages("user_id=erased-other")), 1)
        self.assertIsNotNone(self.memory.get(self.kept))


if __name__ == "__main__":
    unittest.main()
