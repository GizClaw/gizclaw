#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import os
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("redact_diagnostics.py")
SPEC = importlib.util.spec_from_file_location("redact_diagnostics", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
redact_diagnostics = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(redact_diagnostics)


class RedactDiagnosticsTest(unittest.TestCase):
    def test_redacts_credentials_and_preserves_safe_context(self) -> None:
        original = os.environ.get("GIZCLAW_E2E_TEST_API_KEY")
        os.environ["GIZCLAW_E2E_TEST_API_KEY"] = "environment-secret-123"
        self.addCleanup(self._restore_env, original)
        source = """+safe phase=go:chat service=edge status=healthy
Authorization: Bearer header-secret
OPENAI_API_KEY=assignment-secret
{"client_secret":"json-secret","credential": "quoted secret value"}
GET https://provider.example/call?access_token=query-secret&safe=1
POST postgres://runner:database-password@db.internal/gizclaw
provider echoed environment-secret-123 without a key
-----BEGIN PRIVATE KEY-----
private-key-material
-----END PRIVATE KEY-----
"""

        redacted = redact_diagnostics.redact(source)

        self.assertIn("safe phase=go:chat service=edge status=healthy", redacted)
        self.assertIn("safe=1", redacted)
        self.assertGreaterEqual(redacted.count("[REDACTED]"), 8)
        for secret in (
            "header-secret",
            "assignment-secret",
            "json-secret",
            "quoted secret value",
            "query-secret",
            "database-password",
            "environment-secret-123",
            "private-key-material",
        ):
            self.assertNotIn(secret, redacted)

    @staticmethod
    def _restore_env(original: str | None) -> None:
        if original is None:
            os.environ.pop("GIZCLAW_E2E_TEST_API_KEY", None)
        else:
            os.environ["GIZCLAW_E2E_TEST_API_KEY"] = original


if __name__ == "__main__":
    unittest.main()
