#!/usr/bin/env python3
"""Compare two audio input paths using identical recordings.

Each sample is sent to two Workspaces of one Workflow, on the model audio
input path and the ASR path. Acceptance covers routing, nonempty transcript,
text/audio delivery, stream completion and cleanup. Transcripts and replies
are retained as diagnostics; recognition accuracy and answer content are not
scored against language-specific keywords.
"""

import json
import os
import subprocess
import sys


SAMPLES = [
    {"id": "mandarin", "language": "普通话", "voice": "narrator", "text": "三加五等于几？"},
    {"id": "sichuanese", "language": "四川话", "voice": "sichuanese-voice", "text": "一个星期有好多天嘛？"},
    {"id": "cantonese", "language": "粤语", "voice": "cantonese-voice", "text": "法國嘅首都係邊度呀？"},
    {"id": "english", "language": "English", "voice": "english-voice", "text": "What is the capital city of Japan?"},
    {"id": "japanese", "language": "日本語", "voice": "japanese-voice", "text": "日本の首都はどこですか？"},
    {"id": "spanish", "language": "Español", "voice": "spanish-voice", "text": "¿Cuántos días tiene una semana?"},
]

PATHS = ("audio", "asr")
OUTPUT_KEYS = tuple(f"{path}_{field}" for path in PATHS for field in ("transcript", "reply"))
LATENCY_KEYS = ("first_transcript_ms", "first_text_ms", "first_audio_ms", "text_eos_ms")


def parse_outputs(stdout):
    """Collect `name=value` output steps; a value may continue on later lines."""
    values, current = {}, None
    for line in stdout.splitlines():
        key, sep, rest = line.partition("=")
        if sep and key in OUTPUT_KEYS:
            current = key
            values[key] = rest
        elif line.startswith("Giztest "):
            current = None
        elif current is not None:
            values[current] += "\n" + line
    return {key: value.strip() for key, value in values.items()}


def step_evidence(report, step_id):
    for task in report.get("tasks", []):
        for step in task.get("steps", []) or []:
            if step.get("id") == step_id:
                return step
    return {}


def sample_passed(result):
    """Require the actual runner and both path checks, never model semantics."""
    return (
        result["runner_exit_status"] == 0
        and result["document_status"] == "passed"
        and all(
            result["paths"][path]["step_status"] == "passed"
            and bool(result["paths"][path]["transcript"].strip())
            and bool(result["paths"][path]["reply"].strip())
            for path in PATHS
        )
    )


def run_sample(binary, document, artifact_dir, sample):
    report_path = os.path.join(artifact_dir, "runs", f"{sample['id']}.json")
    env = dict(os.environ, GIZCLAW_AUDIO_SAMPLE_VOICE=sample["voice"], GIZCLAW_AUDIO_SAMPLE_TEXT=sample["text"])
    print(f"==> sample={sample['id']} language={sample['language']}", flush=True)
    completed = subprocess.run(
        [binary, "test", "run", "--parallel", "1", "--report", report_path, document],
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
              "runner_exit_status": completed.returncode,
              "document_status": report.get("status", "missing"), "paths": {}}
    for path in PATHS:
        step = step_evidence(report, f"ask_{path}")
        evidence = step.get("evidence", {})
        result["paths"][path] = {
            "step_status": step.get("status", "not_run"),
            "error": step.get("error", ""),
            "transcript": outputs.get(f"{path}_transcript", ""),
            "reply": outputs.get(f"{path}_reply", ""),
            **{key: evidence.get(key) for key in LATENCY_KEYS},
        }
    result["acceptance_status"] = "passed" if sample_passed(result) else "failed"
    return result


def markdown(results):
    lines = ["# Eino audio input comparison", "",
             "`audio`: push-to-talk audio sent to the audio-input chat Model; independent ASR publishes the transcript.",
             "`asr`: the same upstream chat model configured for text input receives independent ASR text.",
             "Acceptance checks the code paths, delivery, stream completion and cleanup. Transcript and reply content are diagnostic only; no language accuracy or answer score is applied.", "",
             "| Sample | Acceptance | Path | Step | Transcript (diagnostic) | Reply (diagnostic) | First transcript ms | First text ms | First audio ms |",
             "| --- | --- | --- | --- | --- | --- | --- | --- | --- |"]
    for result in results:
        for path in PATHS:
            item = result["paths"][path]
            cell = lambda text: (text or f"({item['step_status']})").replace("|", "\\|").replace("\n", " ")
            lines.append(
                f"| {result['language']} | {result['acceptance_status']} | {path} | {item['step_status']} "
                f"| {cell(item['transcript'])} | {cell(item['reply'])} "
                f"| {item['first_transcript_ms']} | {item['first_text_ms']} | {item['first_audio_ms']} |")
    passed = sum(result["acceptance_status"] == "passed" for result in results)
    lines.extend(["", f"- Code-path acceptance: {passed}/{len(results)} samples."])
    return "\n".join(lines) + "\n"


def main():
    binary, document, artifact_dir = sys.argv[1:4]
    selected = set(filter(None, os.environ.get("GIZCLAW_AUDIO_SAMPLES", "").split(",")))
    samples = [sample for sample in SAMPLES if not selected or sample["id"] in selected]
    if not samples:
        raise SystemExit(f"no samples selected from {sorted(sample['id'] for sample in SAMPLES)}")
    os.makedirs(os.path.join(artifact_dir, "runs"), exist_ok=True)
    results = [run_sample(binary, document, artifact_dir, sample) for sample in samples]
    failed = [result["id"] for result in results if result["acceptance_status"] != "passed"]
    with open(os.path.join(artifact_dir, "report.json"), "w", encoding="utf-8") as handle:
        json.dump({"version": "gizclaw.eino-audio-input-comparison/v1", "acceptance": "code-path", "status": "failed" if failed else "passed", "samples": results}, handle,
                  ensure_ascii=False, indent=2)
        handle.write("\n")
    summary = markdown(results)
    with open(os.path.join(artifact_dir, "report.md"), "w", encoding="utf-8") as handle:
        handle.write(summary)
    print(summary)
    if failed:
        raise SystemExit("audio-input code-path checks failed for: " + ", ".join(failed))


if __name__ == "__main__":
    main()
