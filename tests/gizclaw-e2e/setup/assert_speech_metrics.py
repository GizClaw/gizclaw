#!/usr/bin/env python3
"""Validate the persisted histogram contract for the three observability Giztests."""
import json
import math
import sys


def validate(samples):
    required = {
        "genx_asr_first_text_seconds": 2,
        "genx_asr_final_result_seconds": 2,
        "genx_model_first_text_seconds": 2,
        "genx_model_request_duration_seconds": None,
        "genx_tts_first_audio_seconds": 4,
        "genx_realtime_first_text_seconds": 1,
        "genx_realtime_first_audio_seconds": 1,
        "memory_recall_duration_seconds": None,
        "genx_input_end_to_first_output_seconds": None,
    }
    latest = {}
    for sample in samples:
        name = sample["name"]
        if not any(name.startswith(prefix + "_") for prefix in required):
            continue
        labels = sample["labels"]
        allowed = {"provider", "model", "mode", "backend", "flavor", "embedding_model_ref",
                   "rerank_model_ref", "result", "boundary", "event", "role", "le"}
        if set(labels) - allowed:
            raise ValueError(f"unexpected identity or content labels in {name}: {sorted(labels)}")
        if name.startswith("genx_") and not name.startswith("genx_input_end_"):
            if not labels.get("model") or not labels.get("provider"):
                raise ValueError(f"missing resolved model identity: {name} {labels}")
        value = sample["value"]
        if not math.isfinite(value) or value < 0:
            raise ValueError(f"invalid sample {name}: {value}")
        key = (name, tuple(sorted(labels.items())))
        if key not in latest or sample["timestamp_ms"] > latest[key]["timestamp_ms"]:
            latest[key] = sample
    summary = {}
    for name, expected in required.items():
        counts = [s for (n, _), s in latest.items() if n == name + "_count"]
        total = sum(s["value"] for s in counts)
        if not counts or total <= 0 or (expected is not None and total != expected):
            raise ValueError(f"{name} count={total}, expected {expected or 'positive'}")
        groups = []
        for count in counts:
            labels = tuple(sorted(count["labels"].items()))
            summed = latest.get((name + "_sum", labels))
            if not summed or not 0 < summed["value"] <= count["value"] * 120:
                raise ValueError(f"{name} invalid seconds or missing sum: {summed}")
            buckets = [(float(s["labels"]["le"]), s["value"]) for (n, _), s in latest.items()
                       if n == name + "_bucket" and
                       {k: v for k, v in s["labels"].items() if k != "le"} == count["labels"]]
            buckets.sort()
            if not buckets or buckets[-1] != (math.inf, count["value"]):
                raise ValueError(f"{name} missing matching +Inf bucket")
            if any(a[1] > b[1] for a, b in zip(buckets, buckets[1:])):
                raise ValueError(f"{name} buckets are not cumulative")
            groups.append({"labels": count["labels"], "count": count["value"],
                           "mean_ms": round(summed["value"] / count["value"] * 1000, 3)})
        summary[name] = groups
    # Workflow helpers and tool-only completions may add requests without reply text.
    # Every observed first text must still belong to a completed provider request.
    for first in summary["genx_model_first_text_seconds"]:
        count = sum(group["count"] for group in summary["genx_model_request_duration_seconds"]
                    if all(group["labels"].get(k) == v for k, v in first["labels"].items()))
        if count < first["count"]:
            raise ValueError("model first-text count exceeds completed requests")
    return summary


if __name__ == "__main__":
    print(json.dumps(validate(json.load(sys.stdin)), ensure_ascii=False, indent=2))
