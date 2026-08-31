import unittest
from unittest import mock

import mem0_server


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

    def test_standard_request_model_has_no_app_id(self):
        self.assertNotIn("app_id", mem0_server.MemoryCreate.model_fields)
        self.assertEqual(
            set(mem0_server.MemoryCreate.model_fields).intersection(
                {"user_id", "agent_id", "run_id"}
            ),
            {"user_id", "agent_id", "run_id"},
        )


class ConfigurationTest(unittest.TestCase):
    def test_provider_models_and_endpoints_come_from_environment(self):
        environment = {
            "MEM0_LLM_API_KEY": "llm-key",
            "MEM0_LLM_BASE_URL": "https://llm.example/v1",
            "MEM0_LLM_MODEL": "llm-model",
            "MEM0_EMBEDDING_API_KEY": "embedding-key",
            "MEM0_EMBEDDING_BASE_URL": "https://embedding.example/v1",
            "MEM0_EMBEDDING_MODEL": "embedding-model",
        }
        sentinel = object()
        with mock.patch.dict(mem0_server.os.environ, environment, clear=True):
            with mock.patch.object(
                mem0_server.Memory,
                "from_config",
                return_value=sentinel,
            ) as from_config:
                self.assertIs(mem0_server._build_memory(), sentinel)

        config = from_config.call_args.args[0]
        self.assertEqual(
            config["llm"]["config"],
            {
                "api_key": "llm-key",
                "model": "llm-model",
                "openai_base_url": "https://llm.example/v1",
                "temperature": 0.1,
            },
        )
        self.assertEqual(
            config["embedder"]["config"],
            {
                "api_key": "embedding-key",
                "model": "embedding-model",
                "openai_base_url": "https://embedding.example/v1",
            },
        )

if __name__ == "__main__":
    unittest.main()
