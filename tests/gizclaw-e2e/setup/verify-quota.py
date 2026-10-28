#!/usr/bin/env python3
"""Verify real Docker Giztest results and Peer-correlated hook warnings."""

import datetime
import json
from pathlib import Path
import shlex
import sys


def verify(reports):
    report = json.loads((reports / "giztest.json").read_text())
    scenarios = Path(__file__).resolve().parents[1] / "giztest" / "quota"
    expected = {path.name.removesuffix(".giztest.yaml") for path in scenarios.glob("*.giztest.yaml")}
    tasks = report.get("tasks", [])
    if (
        report.get("status") != "passed"
        or len(tasks) != len(expected)
        or {task.get("name") for task in tasks} != expected
        or any(task.get("status") != "passed" for task in tasks)
    ):
        raise ValueError("quota Giztest report did not pass every scenario exactly once")

    warnings = {}
    for line in (reports / "server.log").read_text().splitlines():
        if "peerquota: quota refresh failed; retaining last decision" not in line:
            continue
        fields = dict(item.split("=", 1) for item in shlex.split(line) if "=" in item)
        if fields.get("level") != "WARN" or not fields.get("peer_public_key"):
            raise ValueError("quota refresh failure lacks WARN level or Peer identity")
        warnings.setdefault(fields["peer_public_key"], []).append(fields)

    reasons = {
        "failure": "quota HTTP status 503",
        "malformed": "quota valid_until must be a future timestamp",
        "stale": "quota HTTP status 503",
        "timeout": "quota request failed",
        "denied-stale": "quota HTTP status 503",
    }
    checked = set()
    connection_peers = {}
    for evidence in json.loads((reports / "hook-evidence.json").read_text()):
        for connection in evidence["connections"]:
            connection_peers.setdefault(connection, set()).add(evidence["peer_public_key"])
        mode = evidence["mode"]
        if mode not in reasons:
            continue
        peer_warnings = warnings.get(evidence["peer_public_key"], [])
        if evidence["failures"] < 1 or not peer_warnings:
            raise ValueError(f"{mode}: hook failure did not produce a Peer-correlated warning")
        if not any(warning.get("reason") == reasons[mode] for warning in peer_warnings):
            raise ValueError(f"{mode}: warning does not contain the expected safe failure reason")
        if mode == "timeout" and evidence["canceled"] < 1:
            raise ValueError("timeout: no real HTTP hook cancellation was observed")
        previous = None
        for warning in peer_warnings:
            # A worker can outlive its Giztest Peer. Cleanup then makes usage
            # unavailable; it must remain sanitized and rate-limited too.
            if warning.get("reason") not in {reasons[mode], "cannot read quota usage report"}:
                raise ValueError(f"{mode}: warning contains an unexpected failure reason")
            timestamp = datetime.datetime.fromisoformat(warning["time"].replace("Z", "+00:00"))
            if previous is not None and (timestamp - previous).total_seconds() < 59:
                raise ValueError(f"{mode}: repeated failures bypassed warning rate limiting")
            previous = timestamp
        checked.add(mode)
    if checked != reasons.keys():
        raise ValueError(f"missing hook failure evidence: {sorted(reasons.keys() - checked)}")
    shared = sum(len(peers) > 1 for peers in connection_peers.values())
    if not shared:
        raise ValueError("quota hooks did not reuse any HTTP connection across Peers")
    print(f"Quota Docker E2E: {len(tasks)} Giztests passed; all {len(checked)} hook-failure warning modes verified; {shared} HTTP connections reused across Peers.")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit("usage: verify-quota.py <reports-directory>")
    try:
        verify(Path(sys.argv[1]))
    except (OSError, ValueError, KeyError) as error:
        sys.exit(f"Quota Docker E2E verification failed: {error}")
