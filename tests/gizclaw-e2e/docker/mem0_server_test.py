import contextlib
import asyncio
import io
import json
import os
import tempfile
import threading
import unittest
from concurrent.futures import ThreadPoolExecutor
from types import SimpleNamespace
from unittest import mock

import httpx
import anyio
from fastapi.testclient import TestClient
from openai import APIStatusError, OpenAI
from mem0.exceptions import LLMError

from gizclaw_mem0 import server as mem0_server
from gizclaw_mem0 import schema


class RoutingTest(unittest.TestCase):
    def test_accepts_each_native_self_hosted_entity(self):
        routes = [
            {"user_id": "user"},
            {"agent_id": "agent"},
            {"run_id": "run"},
            {
                "user_id": "user",
                "agent_id": "agent",
                "run_id": "run",
            },
        ]
        for route in routes:
            with self.subTest(route=route):
                self.assertEqual(mem0_server._routing_kwargs(route), route)

    def test_routing_rejects_empty_and_wildcard(self):
        with self.assertRaisesRegex(ValueError, "at least one"):
            mem0_server._routing_kwargs({})
        with self.assertRaisesRegex(ValueError, "wildcard"):
            mem0_server._routing_kwargs({"user_id": "*"})

    def test_routing_ignores_non_entity_request_fields(self):
        self.assertEqual(
            mem0_server._routing_kwargs(
                {
                    "messages": [{"role": "user", "content": "hello"}],
                    "metadata": {"source": "test"},
                    "infer": True,
                    "user_id": "gizclaw-scope-v1:encoded",
                }
            ),
            {"user_id": "gizclaw-scope-v1:encoded"},
        )


class OpenAPIContractTest(unittest.TestCase):
    def test_python_implementation_matches_canonical_contract(self):
        expected = schema.normalize(schema.contract())
        actual = schema.implementation(mem0_server.app)
        for name, model in expected["components"]["schemas"].items():
            if name != "ErrorResponse":
                self.assertEqual(actual["components"]["schemas"][name], model, name)
        self.assertEqual(set(actual["paths"]), set(expected["paths"]))
        for path, methods in expected["paths"].items():
            for method, operation in methods.items():
                runtime = actual["paths"][path][method]
                self.assertEqual(runtime["operationId"], operation["operationId"])
                self.assertEqual(runtime.get("parameters"), operation.get("parameters"))
                self.assertEqual(runtime.get("requestBody"), operation.get("requestBody"))
                success = next(status for status in operation["responses"] if status.startswith("2"))
                self.assertEqual(runtime["responses"][success].get("content"), operation["responses"][success].get("content"))


    def test_standard_request_model_has_no_app_id(self):
        self.assertNotIn("app_id", mem0_server.MemoryCreate.model_fields)
        self.assertEqual(
            set(mem0_server.MemoryCreate.model_fields).intersection({"user_id", "agent_id", "run_id"}),
            {"user_id", "agent_id", "run_id"},
        )


class ArkEmbeddingTest(unittest.TestCase):
    def embedding(self, handler):
        config = SimpleNamespace(
            model="doubao-embedding-vision-251215", embedding_dims=1024,
            api_key="fixture-key", openai_base_url="https://embedding.example/api/v3",
        )
        embedding = mem0_server.ArkMultimodalEmbedding(config)
        embedding.client.close()
        embedding.client = OpenAI(
            api_key="fixture-key", base_url=config.openai_base_url,
            http_client=httpx.Client(transport=httpx.MockTransport(handler)),
        )
        self.addCleanup(embedding.client.close)
        return embedding

    def test_batch_preserves_one_vector_per_text_and_query_instructions(self):
        requests = []

        def handler(request):
            self.assertEqual(request.url.path, "/api/v3/embeddings/multimodal")
            body = json.loads(request.content)
            requests.append(body)
            return httpx.Response(200, json={"data": {"embedding": [len(requests) / 10] * 1024}})

        embedding = self.embedding(handler)
        vectors = embedding.embed_batch(["first fact", "second fact"])
        self.assertEqual([v[0] for v in vectors], [0.1, 0.2])
        self.assertEqual([r["input"] for r in requests], [
            [{"type": "text", "text": "first fact"}],
            [{"type": "text", "text": "second fact"}],
        ])
        self.assertTrue(all(r["instructions"] == embedding.corpus_instructions for r in requests))
        embedding.embed("a question", "search")
        self.assertEqual(requests[-1]["instructions"], embedding.query_instructions)
        self.assertEqual(requests[-1]["dimensions"], 1024)
        self.assertEqual(len(embedding.policy_fingerprint()), 64)

    def test_invalid_vectors_and_provider_errors_fail(self):
        for vector in [[0.1], [True] * 1024, [float("nan")] * 1024, [float("inf")] * 1024]:
            with self.subTest(kind=str(type(vector[0]))):
                embedding = self.embedding(lambda _: httpx.Response(200, content=json.dumps({"data": {"embedding": vector}}), headers={"Content-Type": "application/json"}))
                with self.assertRaisesRegex(RuntimeError, "invalid embedding vector"):
                    embedding.embed("fact")
                self.assertIsNotNone(embedding.last_error)
        embedding = self.embedding(lambda _: httpx.Response(403, json={"error": {"message": "denied"}}))
        with self.assertRaises(APIStatusError):
            embedding.embed("fact")
        self.assertIsNotNone(embedding.last_error)

    def test_integer_zero_is_normalized_for_pgvector_float_arrays(self):
        embedding = self.embedding(lambda _: httpx.Response(200, json={"data": {"embedding": [0, 0.25] * 512}}))
        vector = embedding.embed("fact")
        self.assertEqual(vector[:2], [0.0, 0.25])
        self.assertTrue(all(isinstance(value, float) for value in vector))

    def test_invalid_dimensions_are_rejected_before_client_initialization(self):
        with self.assertRaisesRegex(RuntimeError, "1024 or 2048"):
            mem0_server.ArkMultimodalEmbedding(SimpleNamespace(embedding_dims=1536))

    def test_swallowed_embedding_error_cannot_become_empty_write(self):
        embedding = self.embedding(lambda _: httpx.Response(403, json={"error": {"message": "denied"}}))
        memory = mock.Mock(embedding_model=embedding)

        def swallowed(**kwargs):
            try:
                embedding.embed("fact")
            except Exception:
                # Intentionally mimic the SDK swallowing this provider error.
                pass
            return {"results": []}

        memory.add.side_effect = swallowed
        request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "input"}], user_id="scope")
        with mock.patch.object(mem0_server, "_memory", memory):
            with self.assertRaises(mem0_server.HTTPException) as raised:
                mem0_server.add_memory(request)
        self.assertEqual(raised.exception.status_code, 502)

class ConfigurationTest(unittest.TestCase):
    def native_config(self):
        return {
            "llm": {"provider": "openai", "config": {"model": "llm-model", "api_key": "${MODEL_KEY}"}},
            "embedder": {"provider": "openai", "config": {"model": "embedding-model", "api_key": "${MODEL_KEY}", "embedding_dims": 1024}},
            "vector_store": {"provider": "pgvector", "config": {"connection_string": "${DATABASE_DSN}", "embedding_model_dims": 1024}},
        }

    def test_model_tier_is_consumed_before_native_sdk_config(self):
        for tier in ("fast", "priority"):
            with self.subTest(tier=tier):
                config = self.native_config()
                config["llm"]["config"]["service_tier"] = tier
                memory = SimpleNamespace(llm=SimpleNamespace(client=mock.Mock(), config={"model": "fixture", "api_key": "fixture"}))
                with mock.patch.object(mem0_server.Memory, "from_config", return_value=memory) as constructor:
                    result = mem0_server._configured_memory(config, "", "openai")
                try:
                    self.assertEqual(result.llm.service_tier, tier)
                    self.assertNotIn("service_tier", constructor.call_args.args[0]["llm"]["config"])
                    self.assertEqual(config["llm"]["config"]["service_tier"], tier)
                finally:
                    result.llm.client.close()

    def test_invalid_model_tier_fails_before_native_sdk_initialization(self):
        config = self.native_config()
        config["llm"]["config"]["service_tier"] = "unsupported"
        with mock.patch.object(mem0_server.Memory, "from_config") as constructor:
            with self.assertRaisesRegex(RuntimeError, "llm.config.service_tier"):
                mem0_server._configured_memory(config, "", "openai")
            constructor.assert_not_called()

    def test_config_file_expands_secrets_and_passes_native_sdk_settings(self):
        document = {"memory": self.native_config(), "service": {"api_key": "${SERVICE_KEY}"}}
        with tempfile.NamedTemporaryFile(mode="w", suffix=".json") as source:
            json.dump(document, source)
            source.flush()
            environment = {"MEM0_CONFIG": source.name, "MODEL_KEY": "fixture-model-key", "DATABASE_DSN": "postgresql://fixture", "SERVICE_KEY": "fixture-service-key"}
            with mock.patch.dict(os.environ, environment, clear=True), mock.patch.object(mem0_server.Memory, "from_config") as constructor:
                constructor.return_value.llm.config = {"model": "fixture", "api_key": "fixture"}
                memory = mem0_server._build_memory()
                self.addCleanup(memory.llm.client.close)
                self.assertEqual(mem0_server._service_api_key, "fixture-service-key")
        config = constructor.call_args.args[0]
        self.assertEqual(config["llm"]["config"]["api_key"], "fixture-model-key")
        self.assertEqual(config["vector_store"]["config"]["connection_string"], "postgresql://fixture")
        self.assertIs(config["llm"]["config"]["response_callback"], mem0_server._report_llm_response)

    def test_missing_substitution_and_invalid_shapes_fail_before_sdk_initialization(self):
        documents = [
            {"memory": self.native_config()},
            {"memory": self.native_config(), "service": {"api_key": 123}},
            {"memory": self.native_config(), "service": {"unsupported": True}},
            {"memory": self.native_config(), "service": {"service_tier": "fast"}},
            {"memory": {**self.native_config(), "llm": []}},
            {"memory": {**self.native_config(), "embedder": {"provider": "unknown", "config": {}}}},
        ]
        for index, document in enumerate(documents):
            with self.subTest(index=index), tempfile.NamedTemporaryFile(mode="w") as source:
                json.dump(document, source)
                source.flush()
                environment = {"MEM0_CONFIG": source.name}
                if index:
                    environment.update(MODEL_KEY="fixture-model-key", DATABASE_DSN="postgresql://fixture")
                with mock.patch.dict(os.environ, environment, clear=True), mock.patch.object(mem0_server.Memory, "from_config") as constructor:
                    with self.assertRaises(RuntimeError):
                        mem0_server._build_memory()
                    constructor.assert_not_called()

    def test_ark_config_requires_matching_vector_store_dimensions(self):
        config = self.native_config()
        config["embedder"]["config"]["openai_base_url"] = "https://embedding.example/v3"
        config["vector_store"]["config"]["embedding_model_dims"] = 1536
        with mock.patch.object(mem0_server.Memory, "from_config") as constructor:
            with self.assertRaisesRegex(RuntimeError, "dimensions must match"):
                mem0_server._configured_memory(config, "", "ark_multimodal")
            constructor.assert_not_called()

    def test_service_cannot_override_memory_layout_business_instructions(self):
        config = self.native_config()
        config["custom_instructions"] = "global business policy"
        with mock.patch.object(mem0_server.Memory, "from_config") as constructor:
            with self.assertRaisesRegex(RuntimeError, "MemoryLayout"):
                mem0_server._configured_memory(config, "", "openai")
            constructor.assert_not_called()

    def test_provider_models_and_endpoints_come_from_environment(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_LLM_BASE_URL": "https://llm.example/v1",
            "MEM0_LLM_MODEL": "llm-model",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_EMBEDDING_BASE_URL": "https://embedding.example/v1",
            "MEM0_EMBEDDING_MODEL": "embedding-model",
            "MEM0_EMBEDDING_DIMENSIONS": "1024",
        }
        sentinel = SimpleNamespace(llm=SimpleNamespace(client=mock.Mock(), config={"model": "fixture", "api_key": "fixture"}))
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with mock.patch.object(
                mem0_server.Memory,
                "from_config",
                return_value=sentinel,
            ) as from_config:
                self.assertIs(mem0_server._build_memory(), sentinel)
                self.addCleanup(sentinel.llm.client.close)

        config = from_config.call_args.args[0]
        self.assertEqual(
            config["llm"]["config"],
            {
                "api_key": "llm-key",
                "model": "llm-model",
                "openai_base_url": "https://llm.example/v1",
                "temperature": 0.1,
                "max_tokens": 2000,
                "response_callback": mem0_server._report_llm_response,
            },
        )
        self.assertEqual(
            config["embedder"]["config"],
            {
                "api_key": "embedding-key",
                "model": "embedding-model",
                "embedding_dims": 1024,
                "openai_base_url": "https://embedding.example/v1",
            },
        )
        self.assertEqual(
            config["vector_store"]["config"]["embedding_model_dims"],
            1024,
        )

    def test_embedding_dimensions_must_be_positive_integer(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_EMBEDDING_DIMENSIONS": "invalid",
        }
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with self.assertRaisesRegex(RuntimeError, "positive integer"):
                mem0_server._build_memory()

    def test_pgvector_uses_the_configured_database_and_vector_width(self):
        environment = {
            "MEM0_VECTOR_STORE": "pgvector",
            "MEM0_POSTGRES_DSN": "postgresql://test:test@postgres/locomo",
            "MEM0_COLLECTION_NAME": "locomo",
        }
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            config = mem0_server._vector_store_config(1024)
        self.assertEqual(config["provider"], "pgvector")
        self.assertEqual(config["config"]["connection_string"], environment["MEM0_POSTGRES_DSN"])
        self.assertEqual(config["config"]["embedding_model_dims"], 1024)
        self.assertEqual(config["config"]["collection_name"], "locomo")
        self.assertNotIn("path", config["config"])

    def test_pgvector_does_not_fall_back_when_database_is_missing(self):
        with mock.patch.dict(mem0_server.os.environ, {"MEM0_VECTOR_STORE": "pgvector"}, clear=True):
            with self.assertRaisesRegex(RuntimeError, "MEM0_POSTGRES_DSN"):
                mem0_server._vector_store_config(1024)

    def test_unknown_vector_store_is_rejected(self):
        with mock.patch.dict(mem0_server.os.environ, {"MEM0_VECTOR_STORE": "unknown"}, clear=True):
            with self.assertRaisesRegex(RuntimeError, "qdrant or pgvector"):
                mem0_server._vector_store_config(1024)

    def test_extraction_token_budget_is_configurable(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_LLM_MAX_TOKENS": "8192",
        }
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with mock.patch.object(mem0_server.Memory, "from_config") as constructor:
                constructor.return_value.llm.config = {"model": "fixture", "api_key": "fixture"}
                memory = mem0_server._build_memory()
                self.addCleanup(memory.llm.client.close)
        self.assertEqual(constructor.call_args.args[0]["llm"]["config"]["max_tokens"], 8192)

    def test_invalid_thinking_mode_is_rejected_before_initialization(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_LLM_THINKING": "invalid",
        }
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with mock.patch.object(mem0_server.Memory, "from_config") as constructor:
                with self.assertRaisesRegex(RuntimeError, "MEM0_LLM_THINKING"):
                    mem0_server._build_memory()
                constructor.assert_not_called()

    def test_explicit_thinking_mode_configures_the_native_llm(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_LLM_THINKING": "disabled",
        }
        memory = mock.Mock()
        memory.llm.config = {"api_key": "llm-key", "model": "doubao-model"}
        original_client = memory.llm.client
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with mock.patch.object(mem0_server.Memory, "from_config", return_value=memory):
                self.assertIs(mem0_server._build_memory(), memory)
        original_client.close.assert_called_once_with()
        self.assertIsInstance(memory.llm, mem0_server.ThinkingOpenAILLM)
        self.assertEqual(memory.llm.thinking, "disabled")
        memory.llm.client.close()


class LunaRequestTest(unittest.TestCase):
    def test_luna_request_uses_completion_limit_and_configured_reasoning(self):
        for effort in (None, "none", "low"):
            with self.subTest(effort=effort):
                requests = []

                def respond(request):
                    requests.append(json.loads(request.content))
                    return httpx.Response(200, json={
                        "id": "fixture", "object": "chat.completion", "created": 0, "model": "gpt-6-luna",
                        "choices": [{"index": 0, "finish_reason": "stop", "message": {"role": "assistant", "content": '{"memory":[]}'}}],
                    })

                llm = mem0_server.CompatibleOpenAILLM({"api_key": "fixture", "model": "gpt-6-luna", "max_tokens": 8192, "reasoning_effort": effort})
                llm.client.close()
                llm.client = OpenAI(api_key="fixture", http_client=httpx.Client(transport=httpx.MockTransport(respond)))
                try:
                    llm.generate_response([{"role": "user", "content": "fixture"}], response_format={"type": "json_object"})
                    body = requests[0]
                    self.assertEqual(body["max_completion_tokens"], 8192)
                    self.assertNotIn("max_tokens", body)
                    self.assertEqual(body["reasoning_effort"], effort or "medium")
                    if effort != "none":
                        self.assertNotIn("temperature", body)
                        self.assertNotIn("top_p", body)
                    self.assertNotIn("thinking", body)
                finally:
                    llm.client.close()


class LayoutPolicyTest(unittest.TestCase):
    def test_two_request_policies_reach_native_sdk_without_mutating_shared_config(self):
        requests = []

        def respond(request):
            requests.append(json.loads(request.content))
            return httpx.Response(200, json={
                "id": "fixture", "object": "chat.completion", "created": 0, "model": "fixture-model",
                "choices": [{"index": 0, "finish_reason": "stop", "message": {"role": "assistant", "content": '{"memory":[]}'}}],
            })

        with tempfile.TemporaryDirectory() as directory:
            config = {
                "version": "v1.1", "history_db_path": directory + "/history.db",
                "llm": {"provider": "openai", "config": {"model": "fixture-model", "api_key": "fixture"}},
                "embedder": {"provider": "openai", "config": {"model": "fixture-embedding", "api_key": "fixture", "embedding_dims": 2}},
                "vector_store": {"provider": "qdrant", "config": {"collection_name": "fixture", "path": directory + "/qdrant", "embedding_model_dims": 2}},
            }
            memory = mem0_server._configured_memory(config, "", "openai")
            memory.llm.client.close()
            memory.llm.client = OpenAI(api_key="fixture", http_client=httpx.Client(transport=httpx.MockTransport(respond)))
            memory.embedding_model.client.close()
            memory.embedding_model = mock.Mock(embed=mock.Mock(return_value=[1.0, 0.0]))
            try:
                with mock.patch.object(mem0_server, "_memory", memory), contextlib.redirect_stdout(io.StringIO()):
                    for policy in ("pet-layout-policy", "calendar-layout-policy"):
                        mem0_server.add_memory(mem0_server.MemoryCreate(messages=[{"role": "user", "content": "fixture message"}], user_id=policy, prompt=policy))
                self.assertEqual(len(requests), 2)
                for index, policy in enumerate(("pet-layout-policy", "calendar-layout-policy")):
                    text = requests[index]["messages"][1]["content"]
                    self.assertIn(policy, text)
                    self.assertNotIn(("calendar-layout-policy", "pet-layout-policy")[index], text)
                self.assertIsNone(memory.custom_instructions)
            finally:
                memory.llm.client.close()
                memory.vector_store.client.close()
                memory.close()


class ConcurrentOperationTest(unittest.TestCase):
    def test_id_mutations_resolve_before_matching_purge_can_run(self):
        for mutation in ("update", "delete"):
            with self.subTest(mutation=mutation):
                memory = mock.Mock()
                memory.config.vector_store.provider = "pgvector"
                looked_up, release, purge_started, purged = (threading.Event() for _ in range(4))
                order = []
                record = {"id": "fact", "user_id": "scope", "agent_id": "agent"}

                def get(memory_id):
                    if not looked_up.is_set():
                        order.append("lookup")
                        looked_up.set()
                        if not release.wait(3):
                            raise RuntimeError("lookup was not released")
                    return record

                def purge(**kwargs):
                    order.append("purge")
                    purged.set()

                def remove_scope():
                    purge_started.set()
                    return mem0_server.delete_memories(user_id="scope")

                memory.get.side_effect = get
                memory.update.side_effect = lambda **kwargs: order.append("update")
                memory.delete.side_effect = lambda **kwargs: order.append("delete")
                memory.delete_all.side_effect = purge
                call = (lambda: mem0_server.update_memory("fact", mem0_server.MemoryUpdate(text="updated"))) if mutation == "update" else (lambda: mem0_server.delete_memory("fact"))
                with mock.patch.object(mem0_server, "_memory", memory), ThreadPoolExecutor(max_workers=2) as pool:
                    writer = pool.submit(call)
                    self.assertTrue(looked_up.wait(3))
                    cleaner = pool.submit(remove_scope)
                    try:
                        self.assertTrue(purge_started.wait(3))
                        self.assertFalse(purged.wait(0.1))
                    finally:
                        release.set()
                    writer.result(timeout=3)
                    cleaner.result(timeout=3)
                self.assertEqual(order, ["lookup", mutation, "purge"])
                self.assertEqual(mem0_server._scope_locks, {})

    def test_id_lookup_waits_for_an_in_progress_purge(self):
        memory = mock.Mock()
        memory.config.vector_store.provider = "pgvector"
        entered, release, lookup, started = (threading.Event() for _ in range(4))

        def purge(**kwargs):
            entered.set()
            if not release.wait(3):
                raise RuntimeError("purge was not released")

        def get(memory_id):
            lookup.set()
            return None

        def update():
            started.set()
            try:
                mem0_server.update_memory("gone", mem0_server.MemoryUpdate(text="updated"))
            except mem0_server.HTTPException as error:
                return error.status_code

        memory.delete_all.side_effect = purge
        memory.get.side_effect = get
        with mock.patch.object(mem0_server, "_memory", memory), ThreadPoolExecutor(max_workers=2) as pool:
            cleaner = pool.submit(mem0_server.delete_memories, user_id="scope")
            self.assertTrue(entered.wait(3))
            writer = pool.submit(update)
            try:
                self.assertTrue(started.wait(3))
                self.assertFalse(lookup.wait(0.1))
            finally:
                release.set()
            cleaner.result(timeout=3)
            self.assertEqual(writer.result(timeout=3), 400)
        memory.update.assert_not_called()

    def test_compound_entity_write_finishes_before_overlapping_purge(self):
        for entity in mem0_server._ROUTING_FIELDS:
            with self.subTest(entity=entity):
                memory = mock.Mock()
                memory.config.vector_store.provider = "pgvector"
                entered, release, purge_started, purged = (threading.Event() for _ in range(4))
                records = []

                def add(**kwargs):
                    entered.set()
                    if not release.wait(3):
                        raise RuntimeError("write was not released")
                    records.append("fact")
                    return {"results": [{"id": "fact", "event": "ADD"}]}

                def purge(**kwargs):
                    records.clear()
                    purged.set()

                def delete():
                    purge_started.set()
                    return mem0_server.delete_memories(**{entity: "shared"})

                memory.add.side_effect = add
                memory.delete_all.side_effect = purge
                memory.get.return_value = {"id": "fact"}
                request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "fixture"}],
                                                  infer=False, user_id="shared", agent_id="shared", run_id="shared")
                with mock.patch.object(mem0_server, "_memory", memory), ThreadPoolExecutor(max_workers=2) as pool:
                    writer = pool.submit(mem0_server.add_memory, request)
                    self.assertTrue(entered.wait(3))
                    deleter = pool.submit(delete)
                    try:
                        self.assertTrue(purge_started.wait(3))
                        self.assertFalse(purged.wait(0.1))
                    finally:
                        release.set()
                    writer.result(timeout=3)
                    deleter.result(timeout=3)
                self.assertTrue(purged.is_set())
                self.assertEqual(records, [])
                self.assertEqual(mem0_server._scope_locks, {})

    def test_provider_rate_limit_is_typed_429_without_upstream_credentials(self):
        request = httpx.Request("POST", "https://provider.example/v1/chat/completions")
        upstream = APIStatusError("private-provider-key", response=httpx.Response(429, request=request), body={"code":"ModelAccountTpmRateLimitExceeded"})
        wrapped = LLMError("private-provider-key")
        wrapped.__cause__ = upstream
        memory = mock.Mock()
        memory.config.vector_store.provider = "pgvector"
        memory.add.side_effect = wrapped
        with mock.patch.object(mem0_server,"_memory",memory), mock.patch.object(mem0_server,"_service_api_key",""):
            client = TestClient(mem0_server.app)
            try:
                response = client.post("/memories", json={"messages":[{"role":"user","content":"fixture"}],"user_id":"scope"})
                self.assertEqual(response.status_code,429)
                self.assertEqual(response.json(),{"detail":"Mem0 model provider rate limit exceeded"})
                self.assertNotIn("private-provider-key",response.text)
            finally:
                client.close()

    def test_admission_limit_returns_503_without_queuing_more_backend_work(self):
        entered, release = threading.Event(), threading.Event()
        memory = mock.Mock()
        memory.config.vector_store.provider = "pgvector"
        def blocked(**kwargs):
            entered.set()
            if not release.wait(3):
                raise RuntimeError("test backend was not released")
            return {"results": []}
        memory.add.side_effect = blocked
        async def run():
            with mock.patch.object(mem0_server, "_memory", memory), mock.patch.object(mem0_server, "_request_limiter", anyio.CapacityLimiter(1)), mock.patch.object(mem0_server, "_service_api_key", ""):
                async with httpx.AsyncClient(transport=httpx.ASGITransport(app=mem0_server.app), base_url="http://fixture") as client:
                    body={"messages":[{"role":"user","content":"fixture"}],"user_id":"scope","infer":False}
                    first=asyncio.create_task(client.post("/memories",json=body))
                    try:
                        self.assertTrue(await anyio.to_thread.run_sync(lambda: entered.wait(3)))
                        second=await client.post("/memories",json=body)
                        self.assertEqual(second.status_code,503)
                        self.assertEqual(memory.add.call_count,1)
                    finally:
                        release.set()
                    self.assertEqual((await first).status_code,200)
        asyncio.run(run())

    def test_pg_scopes_overlap_without_crossing_extraction_diagnostics(self):
        barrier = threading.Barrier(2)
        memory = mock.Mock()
        memory.config.vector_store.provider = "pgvector"

        def add(**kwargs):
            content = '{"memory":[]}'
            response = SimpleNamespace(choices=[SimpleNamespace(
                finish_reason="length" if kwargs["user_id"] == "bad-scope" else "stop",
                message=SimpleNamespace(content=content))], usage=None)
            mem0_server._report_llm_response(None, response, None)
            barrier.wait(timeout=3)
            return {"results": []}

        memory.add.side_effect = add
        def observe(scope):
            try:
                mem0_server.add_memory(mem0_server.MemoryCreate(messages=[{"role":"user","content":"fixture"}], user_id=scope, prompt=scope))
                return 200
            except mem0_server.HTTPException as error:
                return error.status_code
        with mock.patch.object(mem0_server, "_memory", memory), contextlib.redirect_stdout(io.StringIO()), ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(observe, ["bad-scope", "good-scope"]))
        self.assertEqual(results, [502, 200])
        self.assertEqual(mem0_server._scope_locks, {})

    def test_ark_provider_failure_is_thread_local(self):
        embedding = mem0_server.ArkMultimodalEmbedding(SimpleNamespace(
            embedding_dims=1024, model="fixture", api_key="fixture", openai_base_url="https://fixture.example"))
        self.addCleanup(embedding.client.close)
        barrier = threading.Barrier(2)
        def state(value):
            embedding.last_error = value
            barrier.wait(timeout=3)
            return embedding.last_error
        with ThreadPoolExecutor(max_workers=2) as pool:
            self.assertEqual(list(pool.map(state, ["failed", None])), ["failed", None])

    def test_same_scope_writes_are_serialized_and_lock_entries_are_released(self):
        memory = mock.Mock()
        memory.config.vector_store.provider = "pgvector"
        entered, release = threading.Event(), threading.Event()
        def first():
            with mem0_server._write_lock({"user_id":"scope"}):
                entered.set()
                self.assertTrue(release.wait(3))
        with mock.patch.object(mem0_server,"_memory",memory), ThreadPoolExecutor(max_workers=2) as pool:
            a = pool.submit(first)
            self.assertTrue(entered.wait(3))
            def second():
                with mem0_server._write_lock({"user_id":"scope"}):
                    return "complete"
            b = pool.submit(second)
            self.assertFalse(b.done())
            release.set()
            a.result(timeout=3)
            self.assertEqual(b.result(timeout=3), "complete")
        self.assertEqual(mem0_server._scope_locks, {})


class AuthenticationTest(unittest.TestCase):
    def test_health_is_public_and_memory_endpoints_require_configured_key(self):
        memory = SimpleNamespace(
            config=SimpleNamespace(vector_store=SimpleNamespace(provider="pgvector"), llm=SimpleNamespace(provider="openai"), custom_instructions="fixture"),
            llm=SimpleNamespace(config=SimpleNamespace(model="fixture-model", max_tokens=8192)),
            embedding_model=SimpleNamespace(config=SimpleNamespace(model="fixture-embedding", embedding_dims=1024)),
            get_all=mock.Mock(return_value={"results": []}),
        )
        with mock.patch.object(mem0_server, "_service_api_key", "fixture-service-key"), mock.patch.object(mem0_server, "_memory", memory):
            client = TestClient(mem0_server.app)
            self.assertEqual(client.get("/health").status_code, 200)
            response = client.get("/memories?user_id=scope", headers={"X-API-Key": "fixture-service-key"})
            self.assertEqual(response.status_code, 200)
            for key in ("", "wrong"):
                self.assertEqual(client.get("/memories?user_id=scope", headers={"X-API-Key": key}).status_code, 401)
            self.assertEqual(memory.get_all.call_count, 1)
            client.close()


class ThinkingRequestTest(unittest.TestCase):
    def test_openai_fast_and_priority_tiers_use_the_sdk_request_parameter(self):
        for tier in ("fast", "priority"):
            with self.subTest(tier=tier):
                requests = []

                def respond(request):
                    requests.append(json.loads(request.content))
                    return httpx.Response(200, json={"id": "fixture", "object": "chat.completion", "created": 0,
                        "model": "fixture", "service_tier": "priority", "choices": [{"index": 0,
                        "finish_reason": "stop", "message": {"role": "assistant", "content": '{"memory":[]}'}}]})

                llm = mem0_server.CompatibleOpenAILLM({"model": "fixture", "api_key": "fixture"}, tier)
                llm.client.close()
                llm.client = OpenAI(api_key="fixture", http_client=httpx.Client(transport=httpx.MockTransport(respond)))
                try:
                    self.assertEqual(llm._get_supported_params()["service_tier"], tier)
                    llm.generate_response([{"role": "user", "content": "fixture"}], response_format={"type": "json_object"})
                    self.assertEqual(requests[0]["service_tier"], tier)
                    self.assertNotIn("thinking", requests[0])
                finally:
                    llm.client.close()

    def test_fast_tier_and_thinking_parameters_are_forwarded_together(self):
        requests = []
        def respond(request):
            requests.append(json.loads(request.content))
            return httpx.Response(200,json={"id":"fixture","object":"chat.completion","created":0,"model":"doubao-model","service_tier":"fast","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":'{"memory":[]}'}}]})
        llm=mem0_server.ThinkingOpenAILLM({"api_key":"fixture","model":"doubao-model"},"disabled","fast")
        llm.client.close()
        llm.client=OpenAI(api_key="fixture",base_url="https://fixture.example/v1",http_client=httpx.Client(transport=httpx.MockTransport(respond)))
        try:
            llm.generate_response([{"role":"user","content":"fixture"}],response_format={"type":"json_object"})
            self.assertEqual(requests[0]["thinking"],{"type":"disabled"})
            self.assertEqual(requests[0]["service_tier"],"fast")
        finally: llm.client.close()

    def test_native_request_forwards_thinking_without_changing_extraction(self):
        for thinking in ("enabled", "disabled"):
            with self.subTest(thinking=thinking):
                requests = []

                def respond(request):
                    requests.append(json.loads(request.content))
                    return httpx.Response(200, json={
                        "id": "test-completion", "object": "chat.completion", "created": 0,
                        "model": "doubao-model",
                        "choices": [{"index": 0, "finish_reason": "stop", "message": {
                            "role": "assistant", "content": '{"facts":["test fact"]}',
                        }}],
                    })

                llm = mem0_server.ThinkingOpenAILLM({
                    "api_key": "llm-key", "model": "doubao-model", "max_tokens": 8192,
                }, thinking)
                llm.client.close()
                llm.client = OpenAI(
                    api_key="llm-key", base_url="https://llm.example/v1",
                    http_client=httpx.Client(transport=httpx.MockTransport(respond)),
                )
                try:
                    messages = [{"role": "user", "content": "test input"}]
                    result = llm.generate_response(messages, response_format={"type": "json_object"})
                    self.assertEqual(json.loads(result), {"facts": ["test fact"]})
                    self.assertEqual(requests[0]["thinking"], {"type": thinking})
                    self.assertEqual(requests[0]["max_tokens"], 8192)
                    self.assertEqual(requests[0]["messages"], messages)
                    self.assertEqual(requests[0]["response_format"], {"type": "json_object"})
                finally:
                    llm.client.close()


class EntityBulkEndpointTest(unittest.TestCase):
    def setUp(self):
        self.memory = mock.Mock()
        patcher = mock.patch.object(mem0_server, "_memory", self.memory)
        patcher.start()
        self.addCleanup(patcher.stop)

    def test_list_memories_filters_by_entity(self):
        self.memory.get_all.return_value = {"results": [{"id": "fact"}]}
        result = mem0_server.list_memories(
            user_id="gizclaw-scope-v1:encoded", top_k=1
        )
        self.assertEqual(result, {"results": [{"id": "fact"}]})
        self.memory.get_all.assert_called_once_with(
            filters={"user_id": "gizclaw-scope-v1:encoded"}, top_k=1
        )

    def test_delete_memories_deletes_one_entity(self):
        result = mem0_server.delete_memories(user_id="gizclaw-scope-v1:encoded")
        self.assertIn("message", result)
        self.memory.delete_all.assert_called_once_with(
            user_id="gizclaw-scope-v1:encoded"
        )

    def test_bulk_endpoints_reject_empty_and_wildcard_entities(self):
        for call in (
            lambda: mem0_server.list_memories(top_k=10),
            lambda: mem0_server.delete_memories(),
            lambda: mem0_server.list_memories(user_id="*", top_k=10),
            lambda: mem0_server.delete_memories(user_id="*"),
        ):
            with self.assertRaises(mem0_server.HTTPException) as raised:
                call()
            self.assertEqual(raised.exception.status_code, 400)
        self.memory.get_all.assert_not_called()
        self.memory.delete_all.assert_not_called()


class ResponseMetadataTest(unittest.TestCase):
    def test_native_sdk_validates_extraction_before_committing_partial_memories(self):
        content = json.dumps({"memory": [{"text": "The favorite drink is tea."},
                                        {"text": "The preferred sport is tennis."}]})
        for thinking in ("", "disabled"):
            for mode in ("truncated", "invalid_json", "missing_callback", "valid"):
                with self.subTest(thinking=thinking, mode=mode), tempfile.TemporaryDirectory() as directory:
                    config = {
                        "version": "v1.1", "history_db_path": directory + "/history.db",
                        "llm": {"provider": "openai", "config": {"model": "fixture", "api_key": "fixture"}},
                        "embedder": {"provider": "openai", "config": {"model": "fixture", "api_key": "fixture", "embedding_dims": 2}},
                        "vector_store": {"provider": "qdrant", "config": {"collection_name": "fixture", "path": directory + "/qdrant", "embedding_model_dims": 2}},
                    }
                    memory = mem0_server._configured_memory(config, thinking, "openai")

                    def respond(request):
                        return httpx.Response(200, json={
                            "id": "fixture", "object": "chat.completion", "created": 0, "model": "fixture",
                            "choices": [{"index": 0, "finish_reason": "length" if mode == "truncated" else "stop",
                                         "message": {"role": "assistant", "content": "prefix " + content if mode == "invalid_json" else content}}],
                        })

                    memory.llm.client.close()
                    memory.llm.client = OpenAI(api_key="fixture", http_client=httpx.Client(transport=httpx.MockTransport(respond)))
                    if mode == "missing_callback":
                        memory.llm.config.response_callback = None
                    memory.embedding_model.client.close()
                    memory.embedding_model = mock.Mock(embed=mock.Mock(return_value=[1.0, 0.0]),
                                                       embed_batch=lambda texts, *args: [[1.0, 0.0] for _ in texts])
                    try:
                        with mock.patch.object(mem0_server, "_memory", memory), mock.patch.object(mem0_server, "_service_api_key", ""), contextlib.redirect_stdout(io.StringIO()):
                            memory.add(messages=[{"role": "user", "content": "Existing memory."}], user_id="scope", infer=False)
                            before = memory.get_all(filters={"user_id": "scope"})
                            client = TestClient(mem0_server.app)
                            try:
                                with mock.patch.object(memory.db, "save_messages", wraps=memory.db.save_messages) as save_messages:
                                    response = client.post("/memories", json={"messages": [{"role": "user", "content": "fixture"}], "user_id": "scope"})
                            finally:
                                client.close()
                            after = memory.get_all(filters={"user_id": "scope"})
                            if mode == "valid":
                                self.assertEqual(response.status_code, 200)
                                self.assertEqual(len(response.json()["results"]), 2)
                                self.assertEqual(len(after["results"]), 3)
                            else:
                                self.assertEqual(response.status_code, 502)
                                self.assertEqual(after, before)
                                save_messages.assert_not_called()
                            self.assertFalse(mem0_server._operation_state.extraction_required)
                    finally:
                        memory.llm.client.close()
                        memory.vector_store.client.close()
                        memory.close()

    def test_diagnostics_do_not_include_model_text_or_credentials(self):
        response = SimpleNamespace(
            choices=[SimpleNamespace(
                finish_reason="stop",
                message=SimpleNamespace(content=json.dumps({
                    "memory": [{"text": "private conversation content"}],
                })),
            )],
            usage=SimpleNamespace(completion_tokens=40),
        )
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            mem0_server._report_llm_response(None, response, {"api_key": "private-key"})
        metadata = json.loads(output.getvalue())
        self.assertEqual(metadata["candidates"], 1)
        self.assertTrue(metadata["valid_json"])
        self.assertNotIn("private conversation", output.getvalue())
        self.assertNotIn("private-key", output.getvalue())

    def test_failed_extraction_is_not_a_successful_empty_write(self):
        memory = mock.Mock()
        memory.add.return_value = {"results": []}
        request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "input"}], user_id="scope")
        with mock.patch.object(mem0_server, "_memory", memory):
            with self.assertRaises(mem0_server.HTTPException) as raised:
                mem0_server.add_memory(request)
        self.assertEqual(raised.exception.status_code, 502)

    def test_empty_extraction_is_valid_when_model_explicitly_returns_empty_memory(self):
        memory = mock.Mock()

        def empty_result(**kwargs):
            response = SimpleNamespace(
                choices=[SimpleNamespace(finish_reason="stop", message=SimpleNamespace(content='{"memory":[]}'))],
                usage=None,
            )
            with contextlib.redirect_stdout(io.StringIO()):
                mem0_server._report_llm_response(None, response, None)
            return {"results": []}

        memory.add.side_effect = empty_result
        request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "hello"}], user_id="scope")
        with mock.patch.object(mem0_server, "_memory", memory):
            self.assertEqual(mem0_server.add_memory(request), {"results": []})

    def test_direct_import_preserves_name_and_checks_persistence(self):
        memory = mock.Mock()
        memory.add.return_value = {"results": [{"id": "fact", "event": "ADD"}]}
        memory.get.return_value = {"id": "fact"}
        request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "direct fact", "name": "Alice"}], user_id="scope", infer=False)
        with mock.patch.object(mem0_server, "_memory", memory):
            mem0_server.add_memory(request)
        self.assertEqual(memory.add.call_args.kwargs["messages"][0]["name"], "Alice")
        memory.get.assert_called_once_with("fact")

    def test_unpersisted_direct_fact_is_rejected(self):
        memory = mock.Mock()
        memory.add.return_value = {"results": [{"id": "missing", "event": "ADD"}]}
        memory.get.return_value = None
        request = mem0_server.MemoryCreate(messages=[{"role": "user", "content": "direct"}], user_id="scope", infer=False)
        with mock.patch.object(mem0_server, "_memory", memory):
            with self.assertRaises(mem0_server.HTTPException) as raised:
                mem0_server.add_memory(request)
        self.assertEqual(raised.exception.status_code, 502)


if __name__ == "__main__":
    unittest.main()
