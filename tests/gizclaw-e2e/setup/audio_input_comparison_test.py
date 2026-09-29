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

    def test_matches_normalizes_case_accents_digits_and_punctuation(self) -> None:
        self.assertTrue(comparison.matches("三 加 五，等于几？", [["3"], ["5"]]))
        self.assertTrue(comparison.matches("¿Cuántos DÍAS tiene una semana?", [["dias"], ["semana"]]))
        self.assertTrue(comparison.matches("一个星期有七天。", [["7"]]))
        self.assertFalse(comparison.matches("SYLLABLE's Daze Teen Una.", [["dias"], ["semana"]]))

    def test_samples_declare_scoring_groups(self) -> None:
        ids = [sample["id"] for sample in comparison.SAMPLES]
        self.assertEqual(len(ids), len(set(ids)))
        for sample in comparison.SAMPLES:
            self.assertTrue(sample["voice"] and sample["text"])
            self.assertTrue(sample["transcript"] and sample["reply"])


if __name__ == "__main__":
    unittest.main()
