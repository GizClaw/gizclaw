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


if __name__ == "__main__":
    unittest.main()
