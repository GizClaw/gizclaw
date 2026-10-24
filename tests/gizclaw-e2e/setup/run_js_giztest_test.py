#!/usr/bin/env python3
"""Ensure native-runner failures cannot become successful aggregate reports."""

import contextlib
import importlib.util
import io
import json
import pathlib
import subprocess
import tempfile
import textwrap
import unittest.mock

SPEC = importlib.util.spec_from_file_location(
    "run_js_giztest", pathlib.Path(__file__).with_name("run_js_giztest.py"))
assert SPEC is not None and SPEC.loader is not None
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


class ScenarioProcessTest(unittest.TestCase):
    def execute(self, status, exit_code):
        with tempfile.TemporaryDirectory() as temporary:
            report = pathlib.Path(temporary) / "report.json"
            report.write_text(json.dumps({"version": "v1", "status": status,
                "tasks": [{"path": "scenario.yaml", "repeat_index": 0,
                           "status": status, "error": "original failure"}]}))
            completed = subprocess.CompletedProcess([], exit_code, "", "")
            with unittest.mock.patch.object(runner.subprocess, "run", return_value=completed):
                return runner.run_document(pathlib.Path("scenario.yaml"), report,
                                           pathlib.Path("index.ts"))

    def test_preserves_scenario_failure(self):
        tasks = self.execute("failed", 4)
        self.assertEqual(tasks[0]["status"], "failed")
        self.assertEqual(tasks[0]["error"], "original failure")

    def test_rejects_success_report_after_process_failure(self):
        tasks = self.execute("passed", 4)
        self.assertEqual(tasks[0]["status"], "failed")
        self.assertIn("disagrees", tasks[0]["error"])

    def test_external_timeout_is_a_failed_task(self):
        with unittest.mock.patch.object(runner.subprocess, "run",
                               side_effect=subprocess.TimeoutExpired([], 600)):
            tasks = runner.run_document(pathlib.Path("scenario.yaml"),
                                        pathlib.Path("unused.json"), pathlib.Path("index.ts"))
        self.assertEqual(tasks[0]["status"], "failed")
        self.assertIn("process deadline", tasks[0]["error"])

    def test_success_remains_success(self):
        self.assertEqual(self.execute("passed", 0)[0]["status"], "passed")

    def test_native_documents_use_distinct_processes_and_preserve_failure(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = pathlib.Path(temporary)
            executable = root / "native-runner"
            executable.write_text(textwrap.dedent("""\
                #!/usr/bin/env python3
                import json, os, pathlib, sys
                document = pathlib.Path(sys.argv[2])
                output = pathlib.Path(sys.argv[sys.argv.index('--output') + 1])
                status = 'failed' if document.name.startswith('failure') else 'passed'
                pathlib.Path(__file__).with_name(str(os.getpid()) + '.pid').write_text(str(document))
                output.write_text(json.dumps({'version': 'v1', 'status': status,
                    'tasks': [{'path': str(document), 'repeat_index': 0,
                               'status': status, 'error': 'native failure' if status == 'failed' else ''}]}))
                sys.exit(4 if status == 'failed' else 0)
                """))
            executable.chmod(0o700)
            output = root / "combined.json"
            with contextlib.redirect_stdout(io.StringIO()):
                result = runner.main(["--executable", str(executable), str(output),
                                      str(root / "success.giztest.yaml"),
                                      str(root / "failure.giztest.yaml")])
            self.assertEqual(result, 4)
            report = json.loads(output.read_text())
            self.assertEqual(report["status"], "failed")
            self.assertEqual(len(report["tasks"]), 2)
            failed = [task for task in report["tasks"] if task["status"] == "failed"]
            self.assertEqual(failed[0]["error"], "native failure")
            self.assertEqual(len(list(root.glob("*.pid"))), 2)


if __name__ == "__main__":
    unittest.main()
