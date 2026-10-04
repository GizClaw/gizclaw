import contextlib
import importlib.util
import io
import pathlib
import unittest

path = pathlib.Path(__file__).with_name("script_quality_report.py")
spec = importlib.util.spec_from_file_location("quality_report", path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class QualityReportTest(unittest.TestCase):
    def test_valid_quality_failure_is_reported_without_dialogue(self):
        report = {"status": "failed", "tasks": [{"name": "werewolf", "steps": [{"operation": "workspace_relay", "evidence": {"quality": {"passed": False, "criteria": [{"id": "progression", "score": 1, "min_score": 3, "evidence_turns": [4], "reason": "private dialogue", "evidence": [{"turn": 4, "quote": "secret text"}]}]}}}]}]}
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertFalse(module.summarize(report))
        self.assertIn("progression: 1/4", output.getvalue())
        self.assertNotIn("private dialogue", output.getvalue())
        self.assertNotIn("secret text", output.getvalue())

    def test_overall_pass_cannot_hide_a_quality_failure(self):
        report = {"status": "passed", "tasks": [{"steps": [{"operation": "workspace_relay", "evidence": {"quality": {"passed": False, "criteria": [{"id": "closure", "score": 0, "min_score": 3, "evidence_turns": [2]}]}}}]}]}
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertFalse(module.summarize(report))

    def test_unavailable_assessment_cannot_pass(self):
        for report in [{"status": "passed", "tasks": []}, {"status": "passed", "tasks": [{"steps": [{"operation": "workspace_relay", "evidence": {}}]}]}]:
            with self.assertRaises(ValueError):
                module.summarize(report)


if __name__ == "__main__":
    unittest.main()
