#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("audio_input_comparison.py")
SPEC = importlib.util.spec_from_file_location("audio_input_comparison", MODULE_PATH)
assert SPEC is not None and SPEC.loader is not None
comparison = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(comparison)


class AudioInputComparisonTest(unittest.TestCase):
    def test_parse_outputs_keeps_multiline_values_and_stops_at_runner_status(self) -> None:
        stdout = "\n".join([
            "Giztest starting: 1 documents, parallel=1",
            "audio_transcript=三加五等于几",
            "audio_reply=等于八。",
            "还有问题吗？",
            "asr_transcript=三加五等于几？",
            "asr_reply=八",
            "Giztest passed: 1 tasks in 7000ms",
        ])
        self.assertEqual(comparison.parse_outputs(stdout), {
            "audio_transcript": "三加五等于几",
            "audio_reply": "等于八。\n还有问题吗？",
            "asr_transcript": "三加五等于几？",
            "asr_reply": "八",
        })

    def passed_result(self):
        return {
            "runner_exit_status": 0,
            "document_status": "passed",
            "paths": {path: {"step_status": "passed", "transcript": "SYLLABLE's Daze Teen Una.",
                             "reply": "An unrelated model answer."} for path in comparison.PATHS},
        }

    def test_acceptance_does_not_score_recognition_or_answer_content(self) -> None:
        self.assertTrue(comparison.sample_passed(self.passed_result()))

    def test_both_paths_require_nonempty_transcripts_and_replies(self) -> None:
        for path in comparison.PATHS:
            for field in ("transcript", "reply"):
                with self.subTest(path=path, field=field):
                    result = self.passed_result()
                    result["paths"][path][field] = "  \n "
                    self.assertFalse(comparison.sample_passed(result))

    def test_runner_document_and_step_failures_cannot_pass(self) -> None:
        for field, value in (("runner_exit_status", 4), ("document_status", "missing"), ("document_status", "failed")):
            with self.subTest(field=field, value=value):
                result = self.passed_result()
                result[field] = value
                self.assertFalse(comparison.sample_passed(result))
        for path in comparison.PATHS:
            result = self.passed_result()
            result["paths"][path]["step_status"] = "not_run"
            self.assertFalse(comparison.sample_passed(result))

    def test_samples_only_define_input_stimuli(self) -> None:
        ids = [sample["id"] for sample in comparison.SAMPLES]
        self.assertEqual(len(ids), len(set(ids)))
        for sample in comparison.SAMPLES:
            self.assertTrue(sample["voice"] and sample["text"])
            self.assertNotIn("transcript", sample)
            self.assertNotIn("reply", sample)


if __name__ == "__main__":
    unittest.main()
