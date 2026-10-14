#!/usr/bin/env python3
"""Sample host and project-scoped Docker resources without changing quotas."""

import argparse
import concurrent.futures
import datetime
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import threading
import time

from redact_diagnostics import redact


def command(args, timeout=8):
    started = time.monotonic()
    try:
        result = subprocess.run(args, capture_output=True, text=True, timeout=timeout)
        return {
            "duration_seconds": round(time.monotonic() - started, 3),
            "exit_status": result.returncode,
            "stdout": redact(result.stdout),
            "stderr": redact(result.stderr),
        }
    except subprocess.TimeoutExpired:
        return {"duration_seconds": round(time.monotonic() - started, 3), "timeout": True}


def file_command(paths):
    # Only constant paths from this module reach the shell inside the container.
    return "\n".join(
        f"printf 'FILE {path}\\n'; cat {path} 2>/dev/null || true" for path in paths
    )


def host_sample():
    if sys.platform == "darwin":
        return {
            "memory": command(["vm_stat"]),
            "swap": command(["sysctl", "-n", "vm.swapusage"]),
            "cpu_disk": command(["iostat", "-c", "2", "-w", "1"]),
        }
    if sys.platform.startswith("linux"):
        return {
            name: command(["cat", path])
            for name, path in {
                "cpu": "/proc/stat",
                "memory": "/proc/meminfo",
                "disk": "/proc/diskstats",
            }.items()
        }
    return {"unavailable": "Host sampling supports macOS and Linux"}


def docker_sample(project):
    result = command(["docker", "ps", "-aq", "--filter", f"label=com.docker.compose.project={project}"])
    identifiers = result.get("stdout", "").split()
    sample = {"api": result}
    if identifiers:
        sample["stats"] = command(
            ["docker", "stats", "--no-stream", "--format", "{{json .}}", *identifiers], 10
        )
        sample["states"] = command(["docker", "inspect", "--format", "{{json .State}}", *identifiers])
    return sample


VM_FILES = [
    "/proc/stat", "/proc/meminfo", "/proc/diskstats",
    "/proc/pressure/cpu", "/proc/pressure/memory", "/proc/pressure/io",
]
CGROUP_FILES = [
    "/sys/fs/cgroup/" + name
    for name in [
        "cpu.stat", "cpu.max", "memory.current", "memory.max", "memory.events",
        "memory.stat", "io.stat", "cpu.pressure", "memory.pressure", "io.pressure",
    ]
]
ROLES = ["server", "edge", "edge2", "tikv-pd", "tikv-leader", "tikv-learner", "postgres"]
PG_QUERY = """
SELECT json_build_object(
  'waits', (SELECT json_agg(a) FROM (
    SELECT state, wait_event_type, wait_event, count(*)
    FROM pg_stat_activity WHERE datname='gizclaw_server'
    GROUP BY state, wait_event_type, wait_event) a),
  'wal', (SELECT row_to_json(w) FROM pg_stat_wal w),
  'io', (SELECT json_agg(a) FROM (
    SELECT backend_type, object, context, reads, read_time, writes, write_time,
           fsyncs, fsync_time FROM pg_stat_io) a)
);
"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--stop-file", type=Path, required=True)
    args = parser.parse_args()
    if not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", args.project):
        parser.error("invalid Docker project")
    os.umask(0o077)
    args.output.mkdir(parents=True, exist_ok=True)
    stop = threading.Event()
    signal.signal(signal.SIGTERM, lambda *_: stop.set())
    signal.signal(signal.SIGINT, lambda *_: stop.set())

    def inside(role, paths):
        return command(["docker", "exec", f"{args.project}-{role}-1", "sh", "-c", file_command(paths)])

    def cgroups():
        with concurrent.futures.ThreadPoolExecutor(max_workers=len(ROLES)) as pool:
            return dict(pool.map(lambda role: (role, inside(role, CGROUP_FILES)), ROLES))

    def sample(name, interval, function):
        with (args.output / (name + ".jsonl")).open("w", buffering=1) as output:
            while not stop.is_set() and not args.stop_file.exists():
                started = time.monotonic()
                row = {"at": datetime.datetime.now(datetime.timezone.utc).isoformat()}
                try:
                    row.update(function())
                except Exception as error:
                    row["collector_error"] = redact(str(error))
                output.write(json.dumps(row) + "\n")
                stop.wait(max(0, interval - (time.monotonic() - started)))

    metadata = {
        "project": args.project,
        "host_platform": sys.platform,
        "docker": command(["docker", "info", "--format", "{{.NCPU}} {{.MemTotal}} {{.Driver}}"]),
        "interval_seconds": {"host": 5, "docker": 5, "vm": 5, "cgroups": 10, "postgres": 5},
    }
    (args.output / "metadata.json").write_text(json.dumps(metadata, indent=2) + "\n")
    with concurrent.futures.ThreadPoolExecutor(max_workers=5) as pool:
        futures = [
            pool.submit(sample, "host", 5, host_sample),
            pool.submit(sample, "docker", 5, lambda: docker_sample(args.project)),
            pool.submit(sample, "vm", 5, lambda: {"snapshot": inside("postgres", VM_FILES)}),
            pool.submit(sample, "cgroups", 10, cgroups),
            pool.submit(sample, "postgres", 5, lambda: {"snapshot": command([
                "docker", "exec", f"{args.project}-postgres-1", "psql", "-U", "gizclaw_bootstrap",
                "-d", "postgres", "-Atc", PG_QUERY,
            ])}),
        ]
        for future in futures:
            future.result()


if __name__ == "__main__":
    main()
