"""Durable, resumable direct observations over the official Mem0 SDK.

The reservation is committed before vector writes. Each candidate is reconciled
by an exact metadata filter, including after an SDK error or lost HTTP reply.
This deliberately does not promise an atomic transaction across Mem0 writes.
"""

import hashlib
import json
import sqlite3
import threading
from contextlib import closing, contextmanager
from pathlib import Path

import psycopg
from fastapi import HTTPException

OBSERVATION = "gizclaw.observation_id"
DIGEST = "gizclaw.observation_digest"
INDEX = "gizclaw.fact_index"
_connections = threading.BoundedSemaphore(16)
_table = "gizclaw_direct_observations"
_ddl = f"""CREATE TABLE IF NOT EXISTS {_table} (
    collection TEXT NOT NULL, routing TEXT NOT NULL, observation TEXT NOT NULL,
    digest TEXT NOT NULL, result_ids TEXT,
    PRIMARY KEY (collection, routing, observation))"""


def _canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)


@contextmanager
def _database(memory, routing):
    config = memory.config.vector_store
    collection = config.config.collection_name
    if config.provider == "pgvector":
        # This pool boundary is independent of Mem0's vector pool: holding a
        # coordinator connection cannot consume the connections SDK writes need.
        with _connections, psycopg.connect(config.config.connection_string, autocommit=True) as db:
            with db.cursor() as cursor:
                cursor.execute("SELECT pg_advisory_lock(%s)", (0x47697A4D656D30,))
                try:
                    cursor.execute(_ddl)
                finally:
                    cursor.execute("SELECT pg_advisory_unlock(%s)", (0x47697A4D656D30,))
                keys = []
                try:
                    for field, value in sorted(routing.items()):
                        key = int.from_bytes(hashlib.sha256(_canonical([collection, field, value]).encode()).digest()[:8], signed=True)
                        cursor.execute("SELECT pg_advisory_lock(%s)", (key,))
                        keys.append(key)
                    yield db, collection, "%s"
                finally:
                    for key in reversed(keys):
                        cursor.execute("SELECT pg_advisory_unlock(%s)", (key,))
    else:
        # Embedded Qdrant has one process owner and the service already holds
        # its memory lock. Keep the reservation beside durable Mem0 history.
        path = Path(memory.config.history_db_path).with_suffix(".observations.db")
        path.parent.mkdir(parents=True, exist_ok=True)
        with closing(sqlite3.connect(path)) as db:
            with _sqlite_cursor(db) as cursor:
                cursor.execute(_ddl)
            yield db, collection, "?"


def _find(memory, routing, observation, index):
    records = memory.vector_store.list(filters={**routing, OBSERVATION: observation, INDEX: index}, top_k=2)
    # The two supported native vector stores return [records, cursor?].
    entries = records[0]
    if len(entries) > 1:
        raise HTTPException(502, "Mem0 direct candidate has duplicate records")
    return memory.get(entries[0].id) if entries else None


def _adopt_legacy(memory, request, routing):
    records = memory.vector_store.list(filters={**routing, OBSERVATION: request.observation_id}, top_k=2)[0]
    if not records:
        return
    # Before keyed batches, the Go adapter persisted one fact with the same
    # canonical digest but no index or reservation. Reconcile it across upgrade.
    if len(records) != 1 or len(request.messages) != 1 or records[0].payload.get(DIGEST) != request.observation_digest:
        raise HTTPException(409, "Observation already exists with a different payload")
    if INDEX not in records[0].payload:
        memory.vector_store.update(vector_id=records[0].id, payload={**records[0].payload, INDEX: 0})


def observe(memory, request, routing):
    payload = request.model_dump(exclude_none=True)
    digest = hashlib.sha256(_canonical(payload).encode()).hexdigest()
    route = _canonical(routing)
    with _database(memory, routing) as (db, collection, parameter):
        params = (collection, route, request.observation_id)
        with _cursor(db) as cursor:
            cursor.execute(f"SELECT digest, result_ids FROM {_table} WHERE collection={parameter} AND routing={parameter} AND observation={parameter}", params)
            saved = cursor.fetchone()
            if saved and saved[0] != digest:
                raise HTTPException(409, "Observation payload changed")
            if not saved:
                _adopt_legacy(memory, request, routing)
                cursor.execute(f"INSERT INTO {_table} (collection,routing,observation,digest) VALUES ({','.join([parameter]*4)})", (*params, digest))
                db.commit()
        completed_ids = json.loads(saved[1]) if saved and saved[1] is not None else None
        results = []
        for index, message in enumerate(request.messages):
            metadata = {**(request.metadata or {}), **(message.metadata or {}),
                        OBSERVATION: request.observation_id, DIGEST: request.observation_digest, INDEX: index}
            record = memory.get(completed_ids[index]) if completed_ids is not None else _find(memory, routing, request.observation_id, index)
            if record is None:
                if completed_ids is not None:
                    raise HTTPException(409, "A completed observation fact was deleted")
                # The official SDK gives each call its own metadata and history.
                # If it raises after persistence, the next retry finds this item.
                memory.add(messages=[message.model_dump(exclude={"metadata"}, exclude_none=True)],
                           metadata=metadata, infer=False, **routing)
                record = _find(memory, routing, request.observation_id, index)
            if not isinstance(record, dict) or record.get("memory") != message.content or any(
                record.get(key) != value for key, value in routing.items()
            ) or any(
                record.get("metadata", {}).get(key) != value for key, value in metadata.items()
            ):
                raise HTTPException(502, "Mem0 direct candidate was not persisted completely")
            results.append(record)
        if completed_ids is None:
            with _cursor(db) as cursor:
                cursor.execute(f"UPDATE {_table} SET result_ids={parameter} WHERE collection={parameter} AND routing={parameter} AND observation={parameter}",
                               (_canonical([record["id"] for record in results]), *params))
            db.commit()
        return {"results": results}


@contextmanager
def _sqlite_cursor(db):
    cursor = db.cursor()
    try:
        yield cursor
    finally:
        cursor.close()


def _cursor(db):
    return db.cursor() if isinstance(db, psycopg.Connection) else _sqlite_cursor(db)


@contextmanager
def purge(memory, routing):
    with _database(memory, routing) as (db, collection, parameter):
        yield
        # Entity purges may cover compound routes. Remove only reservations
        # whose exact native routing contains every selected dimension.
        with _cursor(db) as cursor:
            cursor.execute(f"SELECT routing, observation FROM {_table} WHERE collection={parameter}", (collection,))
            for route, observation in cursor.fetchall():
                values = json.loads(route)
                if all(values.get(key) == value for key, value in routing.items()):
                    cursor.execute(f"DELETE FROM {_table} WHERE collection={parameter} AND routing={parameter} AND observation={parameter}",
                                   (collection, route, observation))
        db.commit()
