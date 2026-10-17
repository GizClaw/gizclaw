#!/usr/bin/env python3

import os
from pathlib import Path
import re
import subprocess
import unittest.mock

import run_script_quality_lane as lane


class QualityLaneTests(unittest.TestCase):
    def run_deadlines(self, overrides=None):
        root = Path(__file__).resolve().parents[3]
        source = (root / "tests/gizclaw-e2e/run_tests.sh").read_text()
        definitions = []
        for name in ("require_positive_seconds", "phase_deadline_seconds", "validate_deadlines"):
            start = source.index(name + "() {")
            end = source.index("\n}\n", start) + 3
            definitions.append(source[start:end])
        assignments = [
            next(line for line in source.splitlines() if line.startswith(name + "="))
            for name in ("full_deadline_seconds", "continue_on_failure")
        ]
        program = "\n".join(definitions + assignments + [
            "validate_deadlines",
            "phase_deadline_seconds giztest:script-quality",
            'echo "$full_deadline_seconds"',
        ])
        environment = {"PATH": os.environ["PATH"], **(overrides or {})}
        return subprocess.run(["bash", "-c", program], env=environment,
                              capture_output=True, text=True, timeout=5)

    def test_default_deadlines_cover_every_sequential_document(self):
        result = self.run_deadlines()
        self.assertEqual(result.returncode, 0, result.stderr)
        quality_seconds, full_seconds = map(int, result.stdout.split())
        root = Path(__file__).resolve().parents[3]
        document_seconds = 0
        for name in lane.FULL_CASES.split():
            source = (root / f"tests/gizclaw-e2e/giztest/script-quality.{name}.giztest.yaml").read_text()
            timeout = re.search(r"^timeout:\s*([1-9][0-9]*)([smh])\s*$", source, re.MULTILINE)
            self.assertIsNotNone(timeout, name)
            document_seconds += int(timeout[1]) * {"s": 1, "m": 60, "h": 3600}[timeout[2]]
        self.assertGreaterEqual(quality_seconds, document_seconds + 45 * 60)
        self.assertGreaterEqual(full_seconds, quality_seconds + 90 * 60)

    def test_quality_deadline_override_and_validation(self):
        valid = self.run_deadlines({"GIZCLAW_E2E_SCRIPT_QUALITY_DEADLINE_SECONDS": "3600"})
        self.assertEqual(valid.returncode, 0, valid.stderr)
        self.assertEqual(valid.stdout.split()[0], "3600")
        for value in ("0", "-1", "35m", "invalid"):
            with self.subTest(value=value):
                invalid = self.run_deadlines({"GIZCLAW_E2E_SCRIPT_QUALITY_DEADLINE_SECONDS": value})
                self.assertEqual(invalid.returncode, 2)
                self.assertIn("giztest:script-quality", invalid.stderr)

    def test_removes_live_lane_identity_and_network_state(self):
        original = {
            "PATH": "/tooling/bin",
            "HOME": "/workspace-user",
            "DOCKER_HOST": "unix:///task/docker.sock",
            "GIZCLAW_E2E_CREDENTIAL_FILE": "/private/providers.env",
            "GIZCLAW_E2E_DOCKER_BASE_IMAGE": "task-go-base:local",
            "GIZCLAW_E2E_MEM0_BASE_IMAGE": "task-memory-base:local",
            "GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES": "1",
            "GIZCLAW_E2E_SCRIPT_QUALITY_ARTIFACT_DIR": "/reports/quality",
            "GIZCLAW_E2E_DOCKER_PROJECT": "old-project",
            "GIZCLAW_E2E_DOCKER_ENV": "/old/docker.env",
            "GIZCLAW_E2E_CONFIG_HOME": "/old/contexts",
            "GIZCLAW_E2E_SERVER_PUBLIC_KEY": "old-server",
            "GIZCLAW_E2E_OPENAI_API_KEY": "old-provider-value",
            "GIZCLAW_TEST_ENDPOINT": "old-server:9000",
            "GIZCLAW_TEST_REGISTRATION_TOKEN": "old-registration",
            "GIZCLAW_E2E_SCRIPT_QUALITY_CASES": "werewolf",
        }
        result = lane.quality_environment(original)
        for name in ("PATH", "HOME", "DOCKER_HOST", *lane.INHERITED_OPTIONS):
            self.assertEqual(result[name], original[name])
        self.assertEqual(result["GIZCLAW_E2E_SCRIPT_QUALITY_CASES"], lane.FULL_CASES)
        for name in original:
            if name.startswith(("GIZCLAW_E2E_", "GIZCLAW_TEST_")):
                if name not in lane.INHERITED_OPTIONS and name != "GIZCLAW_E2E_SCRIPT_QUALITY_CASES":
                    self.assertNotIn(name, result)
        self.assertEqual(original["GIZCLAW_E2E_DOCKER_PROJECT"], "old-project")

    def test_every_quality_document_has_one_complete_lane_case(self):
        root = Path(__file__).resolve().parents[3]
        names = {
            path.name.removeprefix("script-quality.").removesuffix(".giztest.yaml")
            for path in (root / "tests/gizclaw-e2e/giztest").glob("script-quality.*.giztest.yaml")
        }
        cases = lane.FULL_CASES.split()
        self.assertTrue(names)
        self.assertEqual(len(cases), len(set(cases)))
        self.assertEqual(set(cases), names)

    def test_child_failure_reaches_the_complete_gate(self):
        root = Path("/task/repository")
        with unittest.mock.patch.object(lane.subprocess, "call", return_value=7) as child:
            self.assertEqual(lane.run_lane(root, {"GIZCLAW_TEST_ENDPOINT": "old"}), 7)
        child.assert_called_once_with(
            ["bash", str(root / "tests/gizclaw-e2e/run_script_quality_tests.sh")],
            cwd=root,
            env={"GIZCLAW_E2E_SCRIPT_QUALITY_CASES": lane.FULL_CASES},
        )


if __name__ == "__main__":
    unittest.main()
