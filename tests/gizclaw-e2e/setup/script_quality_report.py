#!/usr/bin/env python3
"""Print content-free screenplay verdicts while retaining quoted JSON evidence."""
import json
import sys


def summarize(report):
    if not isinstance(report, dict) or not isinstance(report.get("tasks"), list) or not report["tasks"]:
        raise ValueError("missing quality task report")
    assessed = False
    quality_passed = True
    for task in report["tasks"]:
        for step in task.get("steps", []):
            if step.get("operation") != "workspace_relay":
                continue
            quality = (step.get("evidence") or {}).get("quality")
            if not isinstance(quality, dict):
                continue
            criteria = quality.get("criteria")
            if not isinstance(quality.get("passed"), bool) or not isinstance(criteria, list) or not criteria:
                raise ValueError("incomplete quality result")
            assessed = True
            quality_passed = quality_passed and quality["passed"]
            print(f"{task.get('name')}: {'PASS' if quality.get('passed') is True else 'FAIL'}")
            for criterion in quality.get("criteria", []):
                print(f"  {criterion['id']}: {criterion['score']}/4 (minimum {criterion['min_score']}; turns {criterion.get('evidence_turns', [])})")
    if not assessed:
        raise ValueError("no completed screenplay quality assessment; inspect the bounded operation failure")
    return report.get("status") == "passed" and quality_passed


if __name__ == "__main__":
    try:
        with open(sys.argv[1], encoding="utf-8") as handle:
            success = summarize(json.load(handle))
    except (OSError, ValueError, KeyError, IndexError, TypeError):
        print("screenplay quality report is missing or invalid", file=sys.stderr)
        sys.exit(2)
    sys.exit(0 if success else 1)
