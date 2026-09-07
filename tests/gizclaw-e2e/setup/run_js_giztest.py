#!/usr/bin/env python3
"""Run JS scenario documents in separate native WebRTC process lifetimes."""

import concurrent.futures
import datetime
import json
import pathlib
import subprocess
import sys
import tempfile
import time


def run_document(document, report_path, runner):
    started = time.monotonic()
    command = ["node", "--experimental-strip-types", str(runner), "run",
               str(document), "--parallel", "1", "--output", str(report_path)]
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=600)
        sys.stdout.write(result.stdout)
        sys.stderr.write(result.stderr)
        if result.returncode not in (0, 4):
            raise RuntimeError(f"runner exited with status {result.returncode}")
        report = json.loads(report_path.read_text())
        if (not isinstance(report, dict) or report.get("version") != "v1"
                or report.get("status") not in ("passed", "failed")
                or not isinstance(report.get("tasks"), list)):
            raise ValueError("invalid scenario report")
        tasks = report["tasks"]
        if any(not isinstance(task, dict) or task.get("status") not in
               ("passed", "failed") or not isinstance(task.get("path"), str)
               or not isinstance(task.get("repeat_index"), int) for task in tasks):
            raise ValueError("invalid scenario task report")
        passed = all(task["status"] == "passed" for task in tasks)
        if passed != (report["status"] == "passed") or passed != (result.returncode == 0):
            raise ValueError("scenario report disagrees with exit status")
        return tasks
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        # An external deadline can stop a native call even if Node's event loop
        # is blocked. Keep that failure in the combined acceptance report.
        message = ("native runner exceeded the 600 second process deadline"
                   if isinstance(error, subprocess.TimeoutExpired) else str(error))
        return [{"path": str(document), "name": document.name,
                 "task_id": document.name + "-runner", "repeat_index": 0,
                 "status": "failed", "steps": [], "error": message,
                 "duration_ms": int((time.monotonic() - started) * 1000)}]


def main(arguments):
    output = pathlib.Path(arguments[0]).resolve()
    documents = [pathlib.Path(item).resolve() for item in arguments[1:]]
    runner = pathlib.Path(__file__).resolve().parent.parent / "js/giztest/index.ts"
    started = time.monotonic()
    started_at = datetime.datetime.now(datetime.timezone.utc).isoformat()
    tasks = []
    # Parallelism is between processes; no native PeerConnectionFactory is
    # recycled across unrelated scenario documents in one Node process.
    with tempfile.TemporaryDirectory(prefix="gizclaw-js-scenarios-") as temporary:
        with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
            pending = [executor.submit(run_document, document,
                       pathlib.Path(temporary) / f"{index}.json", runner)
                       for index, document in enumerate(documents)]
            for future in concurrent.futures.as_completed(pending):
                tasks.extend(future.result())
    tasks.sort(key=lambda task: (task["path"], task["repeat_index"]))
    status = "passed" if all(task["status"] == "passed" for task in tasks) else "failed"
    report = {"version": "v1", "status": status, "started_at": started_at,
              "duration_ms": int((time.monotonic() - started) * 1000), "tasks": tasks}
    temporary_output = output.with_suffix(".tmp")
    temporary_output.write_text(json.dumps(report, indent=2) + "\n")
    temporary_output.replace(output)
    print(f"Giztest {status}: {len(tasks)} tasks in {report['duration_ms']}ms", flush=True)
    return 0 if status == "passed" else 4


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
