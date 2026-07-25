import unittest

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


if __name__ == "__main__":
    unittest.main()
