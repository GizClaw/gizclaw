"""Real PGVector contract regressions, requiring no LLM credentials."""

import os
import asyncio
from concurrent.futures import ThreadPoolExecutor
from types import SimpleNamespace
from unittest import mock

from gizclaw_mem0 import server as mem0_server
import unittest
import uuid

from mem0.memory.main import score_and_rank
from mem0.vector_stores.pgvector import PGVector


class PGVectorRegressionTest(unittest.TestCase):
    def setUp(self):
        dsn = os.environ.get("MEM0_POSTGRES_DSN", "")
        if not dsn:
            self.fail("MEM0_POSTGRES_DSN is required; provision the regression PostgreSQL service")
        self.store = PGVector(
            collection_name="regression_" + uuid.uuid4().hex,
            embedding_model_dims=2, connection_string=dsn,
            dbname=None, user=None, password=None, host=None, port=None,
            diskann=False, hnsw=False, minconn=1, maxconn=2,
        )
        self.addCleanup(self.store.connection_pool.close)
        self.addCleanup(self.store.delete_col)

    def test_cold_collection_is_ready_before_parallel_search(self):
        client = mock.Mock()
        client.with_options.return_value = client
        memory = SimpleNamespace(config=SimpleNamespace(vector_store=SimpleNamespace(provider="pgvector")),
                                 llm=SimpleNamespace(client=client), embedding_model=SimpleNamespace(client=client),
                                 vector_store=self.store, close=mock.Mock())

        async def run():
            with mock.patch.object(mem0_server, "_build_memory", return_value=memory), \
                    mock.patch.object(self.store.connection_pool, "close"):
                async with mem0_server._lifespan(mem0_server.app):
                    self.assertTrue(self.store._collection_ensured)
                    with ThreadPoolExecutor(max_workers=5) as pool:
                        results = list(pool.map(lambda _: self.store.search(query="cold read", vectors=[1.0, 0.0]), range(5)))
                    self.assertEqual(results, [[]] * 5)
        asyncio.run(run())

    def test_nearest_vector_survives_mem0_threshold_and_ranking(self):
        nearest, unrelated = str(uuid.uuid4()), str(uuid.uuid4())
        self.store.insert(
            vectors=[[1.0, 0.0], [0.0, 1.0]], ids=[nearest, unrelated],
            payloads=[{"data": "nearest"}, {"data": "unrelated"}],
        )
        native = self.store.search(query="probe", vectors=[1.0, 0.0], top_k=2)
        self.assertEqual([r.id for r in native], [nearest, unrelated])
        self.assertAlmostEqual(native[0].score, 1.0)
        self.assertAlmostEqual(native[1].score, 0.0)
        ranked = score_and_rank(
            [{"id": r.id, "score": r.score, "payload": r.payload} for r in native],
            {}, {}, threshold=0.1, top_k=2,
        )
        self.assertEqual([r["id"] for r in ranked], [nearest])

    def test_vector_and_keyword_search_preserve_scope(self):
        own, other = str(uuid.uuid4()), str(uuid.uuid4())
        self.store.insert(
            vectors=[[1.0, 0.0], [1.0, 0.0]], ids=[own, other],
            payloads=[
                {"data": "needle", "text_lemmatized": "needle", "user_id": "scope-a"},
                {"data": "needle", "text_lemmatized": "needle", "user_id": "scope-b"},
            ],
        )
        filters = {"user_id": "scope-a"}
        semantic = self.store.search(query="needle", vectors=[1.0, 0.0], top_k=10, filters=filters)
        keyword = self.store.keyword_search(query="needle", top_k=10, filters=filters)
        self.assertEqual([r.id for r in semantic], [own])
        self.assertIsNotNone(keyword)
        self.assertEqual([r.id for r in keyword], [own])


if __name__ == "__main__":
    unittest.main()
