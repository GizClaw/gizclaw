"""Incomplete native evidence must never qualify an acceptance run."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent


class ReportTest(unittest.TestCase):
    def run_report(self, root):
        return subprocess.run([sys.executable, "-B", str(ROOT / "report.py"), str(root)], capture_output=True, text=True)

    def test_missing_receipt_keeps_outcomes_unknown(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "inputs").mkdir()
            (root / "inputs/probe.giztest.yaml").write_text(json.dumps({"name": "probe", "repeat": 3, "steps": []}))
            result = self.run_report(root)
            self.assertEqual(result.returncode, 1, result.stderr)
            summary = json.loads((root / "summary.json").read_text())
            self.assertEqual(summary["status"], "FAIL")
            self.assertEqual(summary["native_status"], "missing")
            self.assertEqual(summary["receipt_error"], "native_report_missing")
            self.assertEqual(summary["missing_tasks"], 3)
            self.assertEqual(summary["actual_tasks"], 0)
            self.assertEqual(summary["counts"], {})
            self.assertEqual(summary["tasks"], [])
            self.assertEqual(summary["by_tool_count"], {})
            self.assertFalse((root / "giztest.json").exists())
            self.assertIn("FAIL", (root / "report.html").read_text())

    def test_global_status_cannot_hide_failed_task(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "inputs").mkdir()
            (root / "inputs/probe.giztest.yaml").write_text(json.dumps({"name": "probe", "steps": []}))
            (root / "giztest.json").write_text(json.dumps({"status": "passed", "tasks": [{"name": "probe", "path": "probe.giztest.yaml", "status": "failed", "duration_ms": 1, "steps": []}]}))
            result = self.run_report(root)
            self.assertEqual(result.returncode, 1, result.stderr)
            summary = json.loads((root / "summary.json").read_text())
            self.assertEqual(summary["status"], "FAIL")
            self.assertEqual(summary["counts"], {"failed": 1})
            self.assertEqual(summary["missing_tasks"], 0)

    def test_failed_reply_keeps_actual_request_attribution_without_passing(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "inputs").mkdir()
            document = {"name": "probe.10", "steps": [
                {"id": "turn_0", "client": "peer", "peer_stream": {"mode": "text", "input": "play the selected song"}},
                {"id": "play", "client_rpc": {"method": "client.tool.v0.invoke", "tool": "audioplayer.play"}, "expect": {
                    "/requests": {"count": 1}, "/requests/0/tool": {"equals": "audioplayer.play"},
                    "/requests/0/tool_enum": {"equals": 14}, "/requests/0/args": {"equals": {"index": 1}},
                    "/requests/0/effective_index": {"equals": 1},
                }},
            ]}
            (root / "inputs/probe.giztest.yaml").write_text(json.dumps(document))
            request = {"tool": "audioplayer.play", "tool_enum": 14, "args": {"index": 1}, "effective_index": 1}
            task = {"name": "probe.10", "path": "probe.giztest.yaml", "status": "failed", "duration_ms": 1000,
                    "steps": [{"id": "turn_0", "operation": "peer_stream", "client": "peer", "status": "failed",
                               "started_at": "2026-01-01T00:00:00Z", "duration_ms": 1000, "evidence": {"terminal_errors": 1}}],
                    "cleanup": [{"id": "audit_play", "evidence": {"requests": [
                        dict(request, received_at_unix_ms=1767225600100),
                        dict(request, received_at_unix_ms=1767225601200),
                    ]}}]}
            (root / "giztest.json").write_text(json.dumps({"status": "failed", "tasks": [task]}))
            result = self.run_report(root)
            self.assertEqual(result.returncode, 1, result.stderr)
            summary = json.loads((root / "summary.json").read_text())
            self.assertEqual(summary["status"], "FAIL")
            self.assertEqual(summary["counts"], {"failed": 1})
            metrics = summary["tasks"][0]
            self.assertEqual(metrics["correct_actions"], 1)
            self.assertEqual(metrics["parameter_correct_actions"], 1)
            self.assertEqual(metrics["target_correct_actions"], 1)
            self.assertEqual(metrics["extra_or_wrong_actions"], 1)
            self.assertEqual(metrics["observed_turns"], 0)
            self.assertEqual(json.loads((root / "giztest.json").read_text())["tasks"][0], task)

    def test_explicit_device_api_after_reply_has_its_own_request_window(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "inputs").mkdir()
            def check(identifier, indexes):
                expect = {"/requests": {"count": len(indexes)}}
                for n, index in enumerate(indexes):
                    for key, value in {"tool": "audioplayer.play", "tool_enum": 14, "args": {"index": index}, "effective_index": index}.items():
                        expect[f"/requests/{n}/{key}"] = {"equals": value}
                return {"id": identifier, "client_rpc": {"method": "client.tool.v0.invoke", "tool": "audioplayer.play"}, "expect": expect}
            document = {"name": "probe.10", "steps": [
                {"id": "turn_0", "client": "peer", "peer_stream": {"mode": "text", "input": "play music"}},
                check("model_play", [0]),
                {"id": "direct_play", "http": {"method": "POST", "path": "/gizclaw/v1/device/tool/v0/invoke"}},
                check("direct_received", [0, 1]),
            ]}
            (root / "inputs/probe.giztest.yaml").write_text(json.dumps(document))
            task = {"name": "probe.10", "path": "probe.giztest.yaml", "status": "passed", "duration_ms": 2200,
                    "steps": [
                        {"id": "turn_0", "operation": "peer_stream", "status": "passed", "started_at": "2026-01-01T00:00:00Z", "duration_ms": 400, "evidence": {"user_input": "play music"}},
                        {"id": "direct_play", "operation": "http", "status": "passed", "started_at": "2026-01-01T00:00:02Z", "duration_ms": 200},
                    ], "cleanup": [{"id": "audit_play", "evidence": {"requests": [
                        {"tool": "audioplayer.play", "tool_enum": 14, "args": {"index": 0}, "effective_index": 0, "received_at_unix_ms": 1767225600100},
                        {"tool": "audioplayer.play", "tool_enum": 14, "args": {"index": 1}, "effective_index": 1, "received_at_unix_ms": 1767225602050},
                    ]}}]}
            (root / "giztest.json").write_text(json.dumps({"status": "passed", "tasks": [task]}))
            result = self.run_report(root)
            self.assertEqual(result.returncode, 0, result.stderr)
            metrics = json.loads((root / "summary.json").read_text())["tasks"][0]
            self.assertEqual(metrics["correct_actions"], 2)
            self.assertEqual(metrics["extra_or_wrong_actions"], 0)
            self.assertEqual(metrics["observed_turns"], 1)
            self.assertEqual(metrics["attempted_turns"], 1)


if __name__ == "__main__":
    unittest.main()
