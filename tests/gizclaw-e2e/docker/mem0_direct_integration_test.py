"""Real SDK, embedding and PGVector partial-write/restart regression.

Only the failure boundary is injected; every successful candidate uses the
configured real Mem0 SDK/model/database. Independent processes prove the
reservation and lock do not depend on the HTTP worker's in-memory state.
"""

import os
import uuid
import unittest
from concurrent.futures import ProcessPoolExecutor
from multiprocessing import get_context
from unittest import mock

from dotenv import dotenv_values
from gizclaw_mem0 import server


def _memory():
    credentials = dotenv_values(os.environ["MEM0_CREDENTIAL_FILE"])
    os.environ["VOLC_ARK_API_KEY"] = credentials["GIZCLAW_E2E_VOLC_ARK_API_KEY"]
    return server._build_memory()


def _close(memory):
    memory.llm.client.close()
    memory.embedding_model.client.close()
    memory.vector_store.connection_pool.close()
    memory.close()


def _retry(payload):
    memory = _memory()
    try:
        request = server.MemoryCreate(**payload)
        return server.direct.observe(memory, request, {"user_id": request.user_id})
    finally:
        _close(memory)


class RealDirectIntegrationTest(unittest.TestCase):
    def test_partial_retry_across_processes_and_purge(self):
        memory = _memory()
        self.addCleanup(_close, memory)
        route = {"user_id": "direct-regression-" + uuid.uuid4().hex}
        request = server.MemoryCreate(**route, infer=False, observation_id="turn", observation_digest="a"*64,
            messages=[{"role":"user", "content":"Marzipan prefers dried salmon", "metadata":{"kind":"user"}},
                      {"role":"assistant", "content":"I confirmed Marzipan's salmon preference", "metadata":{"kind":"assistant"}}])
        def purge():
            with server.direct.purge(memory, route):
                memory.delete_all(**route)
        self.addCleanup(purge)
        original_add = memory.add
        count = 0
        def fail_second(**kwargs):
            nonlocal count
            count += 1
            if count == 2:
                raise RuntimeError("injected failure before second candidate")
            return original_add(**kwargs)
        with mock.patch.object(memory, "add", side_effect=fail_second):
            with self.assertRaises(RuntimeError):
                server.direct.observe(memory, request, route)
        persisted = memory.get_all(filters=route, top_k=10)["results"]
        self.assertEqual(len(persisted), 1)
        # New SDK instances in new processes share only persistent PostgreSQL.
        with ProcessPoolExecutor(max_workers=3, mp_context=get_context("spawn")) as pool:
            results = list(pool.map(_retry, [request.model_dump()]*3))
        self.assertTrue(all(result == results[0] for result in results))
        persisted = memory.get_all(filters=route, top_k=10)["results"]
        self.assertEqual(len(persisted), 2)
        self.assertEqual({r["metadata"]["kind"] for r in persisted}, {"user", "assistant"})
        changed = request.model_copy(update={"observation_digest":"b"*64})
        with self.assertRaises(server.HTTPException) as raised:
            server.direct.observe(memory, changed, route)
        self.assertEqual(raised.exception.status_code, 409)
        purge()
        self.assertEqual(memory.get_all(filters=route, top_k=10)["results"], [])
        legacy = server.MemoryCreate(**route, infer=False, observation_id="legacy", observation_digest="c"*64,
            messages=[{"role":"user", "content":"Legacy direct fact", "metadata":{"kind":"legacy"}}])
        native = original_add(messages=[{"role":"user", "content":"Legacy direct fact"}], infer=False,
            metadata={"kind":"legacy", server.direct.OBSERVATION:"legacy", server.direct.DIGEST:"c"*64}, **route)
        adopted = server.direct.observe(memory, legacy, route)
        self.assertEqual(adopted["results"][0]["id"], native["results"][0]["id"])
        self.assertEqual(len(memory.get_all(filters=route, top_k=10)["results"]), 1)
        purge()
        print("Real direct regression: partial 1/2 -> three process retries 2/2, stable IDs, conflict and purge passed", flush=True)


if __name__ == "__main__":
    unittest.main()
