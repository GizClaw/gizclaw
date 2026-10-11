"""Summarize local run-state evidence without conflating logs and tasks."""
import collections
import json
import pathlib
import sys


def percentile(values, fraction):
    if not values:
        return None
    values = sorted(values)
    return values[min(len(values) - 1, int((len(values) - 1) * fraction))]


def timings(events):
    values = [event["duration_ms"] for event in events]
    return {
        "count": len(values),
        "p50_ms": percentile(values, .5),
        "p99_ms": percentile(values, .99),
        "max_ms": max(values, default=None),
        "errors": dict(collections.Counter(event.get("error") for event in events if event.get("error"))),
    }


def summarize(root):
    summary = json.loads((root / "summary.json").read_text())
    report = json.loads((root / "giztest.json").read_text())
    events = json.loads((root / "driver.json").read_text())
    samples = [json.loads(line) for line in (root / "pool.jsonl").read_text().splitlines()]
    logs = [json.loads(line) for line in (root / "permissions.jsonl").read_text().splitlines()]
    decoder = json.JSONDecoder()
    remaining = (root / "pg-activity.jsonl").read_text().strip()
    pg = []
    while remaining:
        value, end = decoder.raw_decode(remaining)
        pg.append(value)
        remaining = remaining[end:].lstrip()
    run_events = [event for event in events if event["application"].endswith("_runs")]
    windows = {}
    for sample in samples:
        window = windows.setdefault(sample["phase"], [sample["time"], sample["time"]])
        window[1] = sample["time"]
    operations = {}
    for op in sorted({event["operation"] for event in run_events}):
        operations[op] = timings([event for event in run_events if event["operation"] == op])
    phases = {}
    for phase, (start, end) in windows.items():
        selected = [event for event in run_events if start <= event["time"] <= end]
        phases[phase] = {op: timings([event for event in selected if event["operation"] == op]) for op in ("connect", "close", "run_state", "run_state_rows_close")}
    failed_steps = collections.Counter()
    voice = collections.defaultdict(list)
    for task in report["tasks"]:
        for step in task.get("steps") or []:
            if step["status"] == "failed":
                failed_steps[(step["id"], step.get("error", ""))] += 1
            evidence = step.get("evidence") or {}
            if "voice" in step["id"] and step["status"] == "passed":
                voice[step["id"]].append(evidence)
    summary.update({
        "path": str(root),
        "status": report["status"],
        "task_status": dict(collections.Counter(task["status"] for task in report["tasks"])),
        "failed_steps": [{"step": key[0], "error": key[1], "count": count} for key, count in failed_steps.items()],
        "voice": {key: {"passed": len(values), "p99_first_text_after_input_ms": percentile([value["first_text_ms"] - value["input_ms"] for value in values if "first_text_ms" in value], .99), "p99_first_audio_after_input_ms": percentile([value["first_audio_ms"] - value["input_ms"] for value in values if "first_audio_ms" in value], .99), "audio_integrity_violations": sum(value.get("audio_integrity", {}).get("violations", 0) for value in values)} for key, values in voice.items()},
        "permission_failures": {stage: {"records": len(selected), "timeout_true_canceled_false": sum(log.get("timeout") is True and log.get("canceled") is False for log in selected)} for stage in sorted({log.get("stage") for log in logs if log.get("stage")}) for selected in [[log for log in logs if log.get("stage") == stage]]},
        "pool_peak": {key: max(sample["pool"][key] for sample in samples) for key in ("OpenConnections", "InUse", "Idle", "WaitCount", "WaitDuration", "MaxIdleClosed", "MaxIdleTimeClosed")},
        "goroutines_peak": max(sample["goroutines"] for sample in samples),
        "operations": operations,
        "phases": phases,
        "pg_lock_samples": sum(activity.get("wait_event_type") == "Lock" for sample in pg for activity in sample["activity"]),
        "pg_idle_in_transaction_samples": sum(activity.get("state") == "idle in transaction" for sample in pg for activity in sample["activity"]),
        "pg_blocked_samples": sum(bool(activity.get("blockers")) for sample in pg for activity in sample["activity"]),
    })
    (root / "analysis.json").write_text(json.dumps(summary, indent=2) + "\n")
    return summary


if __name__ == "__main__":
    results = [summarize(pathlib.Path(value)) for value in sys.argv[1:]]
    print(json.dumps(results, indent=2))
