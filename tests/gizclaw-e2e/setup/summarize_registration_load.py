#!/usr/bin/env python3
"""Summarize real Giztest registration steps and sampled PostgreSQL waits."""

import datetime
import json
import math
import pathlib
import sys


def summarize(root):
    rows = []
    for report_path in sorted(root.glob("*/giztest.json")):
        report = json.loads(report_path.read_text())
        waits = []
        raw = (report_path.parent / "pg-waits.jsonl").read_text()
        decoder = json.JSONDecoder()
        offset = 0
        while offset < len(raw):
            if raw[offset].isspace():
                offset += 1
                continue
            sample, offset = decoder.raw_decode(raw, offset)
            waits.append(sample)
        for phase in ("register", "retry"):
            steps = []
            for task in report["tasks"]:
                for step in task["steps"]:
                    if step["id"] == phase or phase == "retry" and step["id"].startswith("retry_") and step["id"] != "retry_ready":
                        steps.append(step)
            if not steps:
                continue
            start = min(step["start_offset_ms"] for step in steps)
            end = max(step["start_offset_ms"] + step["duration_ms"] for step in steps)
            run_start = datetime.datetime.fromisoformat(report["started_at"])
            phase_waits = [sample for sample in waits if start <= (datetime.datetime.fromisoformat(sample["time"]) - run_start).total_seconds() * 1000 <= end]
            durations = sorted(step["duration_ms"] for step in steps)
            errors = [step.get("error", step["status"]) for step in steps if step["status"] != "passed"]
            row = {
                "scenario": report_path.parent.name,
                "phase": phase,
                "requests": len(steps),
                "errors": errors,
                "throughput_per_second": round((len(steps) - len(errors)) * 1000 / max(end - start, 1), 2),
                **{f"p{p}_ms": durations[min(len(durations) - 1, math.ceil(p / 100 * len(durations)) - 1)] for p in (50, 95, 99)},
                "max_ms": max(durations),
                "max_sampled_lock_waiters": max((sum(a["wait_event_type"] == "Lock" for a in sample["activity"]) for sample in phase_waits), default=0),
                "max_sampled_query_wait_ms": round(max((a["query_ms"] for sample in phase_waits for a in sample["activity"] if a["wait_event_type"] == "Lock"), default=0), 2),
                "max_sampled_transaction_ms": round(max((a["transaction_ms"] or 0 for sample in phase_waits for a in sample["activity"]), default=0), 2),
                "lock_queries": sorted({a["query"] for sample in phase_waits for a in sample["activity"] if a["wait_event_type"] == "Lock"}),
            }
            rows.append(row)
    (root / "summary.json").write_text(json.dumps(rows, indent=2) + "\n")
    print(json.dumps(rows, indent=2))


if __name__ == "__main__":
    summarize(pathlib.Path(sys.argv[1]))
