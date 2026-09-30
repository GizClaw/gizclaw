#!/usr/bin/env python3
"""Run and score benchmark.eino-audio-input-comparison over a fixed sample set.

Each sample is one synthesized question. The Giztest document sends the same
recording to two Workspaces of one Workflow, one on the model audio input path
and one on the asr path, prints both transcripts and replies, and reports
latency evidence. This script runs one
document per sample, scores every transcript and reply against keyword groups,
and writes report.json and report.md to the artifact directory.
"""

import json
import os
import subprocess
import sys
import unicodedata

# Every keyword group must match; any alternative inside a group matches it.
SAMPLES = [
    {"id": "mandarin", "language": "普通话", "voice": "narrator", "text": "三加五等于几？",
     "transcript": [["3"], ["5"]], "reply": [["8"]]},
    {"id": "sichuanese", "language": "四川话", "voice": "sichuanese-voice", "text": "一个星期有好多天嘛？",
     "transcript": [["星期"], ["好多", "多少"]], "reply": [["7"]]},
    {"id": "cantonese", "language": "粤语", "voice": "cantonese-voice", "text": "法國嘅首都係邊度呀？",
     "transcript": [["法國", "法国"], ["首都"], ["嘅", "係", "系", "邊度", "边度"]], "reply": [["巴黎", "paris"]]},
    {"id": "english", "language": "English", "voice": "english-voice", "text": "What is the capital city of Japan?",
     "transcript": [["capital"], ["japan"]], "reply": [["tokyo", "东京", "東京"]]},
    {"id": "japanese", "language": "日本語", "voice": "japanese-voice", "text": "日本の首都はどこですか？",
     "transcript": [["首都", "しゅと"], ["日本", "にほん", "にっぽん"]], "reply": [["東京", "东京", "とうきょう", "tokyo"]]},
    {"id": "spanish", "language": "Español", "voice": "spanish-voice", "text": "¿Cuántos días tiene una semana?",
     "transcript": [["dias"], ["semana"]], "reply": [["7", "siete"]]},
]

PATHS = ("audio", "asr")
OUTPUT_KEYS = tuple(f"{path}_{field}" for path in PATHS for field in ("transcript", "reply"))
LATENCY_KEYS = ("first_transcript_ms", "first_text_ms", "first_audio_ms", "text_eos_ms")
CHINESE_DIGITS = str.maketrans("零〇一二两三四五六七八九", "001223456789")


def normalize(text):
    text = unicodedata.normalize("NFKD", text.casefold())
    text = "".join(ch for ch in text if not unicodedata.combining(ch))
    text = text.translate(CHINESE_DIGITS)
    return "".join(ch for ch in text if not (ch.isspace() or unicodedata.category(ch).startswith("P")))


def matches(text, groups):
    value = normalize(text)
    return all(any(normalize(option) in value for option in group) for group in groups)


def parse_outputs(stdout):
    """Collect `name=value` output steps; a value may continue on later lines."""
    values, current = {}, None
    for line in stdout.splitlines():
        key, sep, rest = line.partition("=")
        if sep and key in OUTPUT_KEYS:
            current = key
            values[key] = rest
        elif line.startswith("Giztest "):
            # The runner's own status lines end the previous value.
            current = None
        elif current is not None:
            values[current] += "\n" + line
    return {key: value.strip() for key, value in values.items()}


def step_evidence(report, step_id):
    for task in report.get("tasks", []):
        for step in task.get("steps", []):
            if step.get("id") == step_id:
                return step
    return {}


def run_sample(binary, document, artifact_dir, sample):
    report_path = os.path.join(artifact_dir, "runs", f"{sample['id']}.json")
    env = dict(os.environ, GIZCLAW_AUDIO_SAMPLE_VOICE=sample["voice"], GIZCLAW_AUDIO_SAMPLE_TEXT=sample["text"])
    print(f"==> sample={sample['id']} language={sample['language']}", flush=True)
    completed = subprocess.run(
        [binary, "test", "run", "--parallel", "1", "--output", report_path, document],
        env=env, capture_output=True, text=True)
    with open(os.path.join(artifact_dir, "runs", f"{sample['id']}.log"), "w", encoding="utf-8") as handle:
        handle.write(completed.stdout)
        handle.write(completed.stderr)
    outputs = parse_outputs(completed.stdout)
    report = {}
    if os.path.isfile(report_path):
        with open(report_path, encoding="utf-8") as handle:
            report = json.load(handle)
    result = {"id": sample["id"], "language": sample["language"], "voice": sample["voice"], "text": sample["text"],
              "document_status": report.get("status", "missing"), "paths": {}}
    for path in PATHS:
        step = step_evidence(report, f"ask_{path}")
        evidence = step.get("evidence", {})
        transcript = outputs.get(f"{path}_transcript", "")
        reply = outputs.get(f"{path}_reply", "")
        result["paths"][path] = {
            "step_status": step.get("status", "not_run"),
            "error": step.get("error", ""),
            "transcript": transcript,
            "reply": reply,
            "transcript_ok": bool(transcript) and matches(transcript, sample["transcript"]),
            "reply_ok": bool(reply) and matches(reply, sample["reply"]),
            **{key: evidence.get(key) for key in LATENCY_KEYS},
        }
    return result


def markdown(results):
    lines = ["# Eino audio input comparison", "",
             "`audio`: push-to-talk audio sent to the audio-input chat Model, which transcribes in the same reply.",
             "`asr`: streaming ASR first, then the same Model with text.", "",
             "| Sample | Path | Transcript | ✓ | Reply | ✓ | First transcript ms | First text ms | First audio ms |",
             "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"]
    for result in results:
        for path in PATHS:
            item = result["paths"][path]
            cell = lambda text: (text or f"({item['step_status']})").replace("|", "\\|").replace("\n", " ")
            lines.append(
                f"| {result['language']} | {path} | {cell(item['transcript'])} | {'✅' if item['transcript_ok'] else '❌'} "
                f"| {cell(item['reply'])} | {'✅' if item['reply_ok'] else '❌'} "
                f"| {item['first_transcript_ms']} | {item['first_text_ms']} | {item['first_audio_ms']} |")
    lines.append("")
    for path in PATHS:
        transcripts = sum(result["paths"][path]["transcript_ok"] for result in results)
        replies = sum(result["paths"][path]["reply_ok"] for result in results)
        lines.append(f"- `{path}`: transcript {transcripts}/{len(results)}, reply {replies}/{len(results)}")
    return "\n".join(lines) + "\n"


def main():
    binary, document, artifact_dir = sys.argv[1:4]
    selected = set(filter(None, os.environ.get("GIZCLAW_AUDIO_SAMPLES", "").split(",")))
    samples = [sample for sample in SAMPLES if not selected or sample["id"] in selected]
    if not samples:
        raise SystemExit(f"no samples selected from {sorted(sample['id'] for sample in SAMPLES)}")
    os.makedirs(os.path.join(artifact_dir, "runs"), exist_ok=True)
    results = [run_sample(binary, document, artifact_dir, sample) for sample in samples]
    with open(os.path.join(artifact_dir, "report.json"), "w", encoding="utf-8") as handle:
        json.dump({"version": "gizclaw.eino-audio-input-comparison/v1", "samples": results}, handle,
                  ensure_ascii=False, indent=2)
        handle.write("\n")
    summary = markdown(results)
    with open(os.path.join(artifact_dir, "report.md"), "w", encoding="utf-8") as handle:
        handle.write(summary)
    print(summary)
    # The audio-input path is the feature under qualification; the ASR path is
    # the baseline it is compared against.
    failed = [result["id"] for result in results
              if not (result["paths"]["audio"]["transcript_ok"] and result["paths"]["audio"]["reply_ok"])]
    if failed:
        raise SystemExit("audio-input path missed expected transcript or reply for: " + ", ".join(failed))


if __name__ == "__main__":
    main()
