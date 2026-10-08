"""Verify local Giztest cleanup and delayed native profiling receipts."""
import datetime
import hashlib
import json
import pathlib
import re
import shutil
import sqlite3
import subprocess
import sys

root, run = map(pathlib.Path, sys.argv[1:])
reports = run / "reports"
profile_root = run / "runtime/server/data/objects/profiling"
shutil.copytree(profile_root, reports / "profiles", dirs_exist_ok=True)
errors = []
summary = {"profiles": [], "reports": [], "ownership": {}, "tasks": []}
client_short_keys = set()
report_names = [f"first-response-{i}.json" for i in range(1, 4)]
report_names += ["lifecycle.json", "slow-tts.json"]
for name in report_names:
    report = json.loads((reports / name).read_text())
    expected = 2 if name == "slow-tts.json" else 12
    if report["status"] != "passed" or len(report["tasks"]) != expected:
        errors.append(f"{name}: expected {expected} passed tasks")
    for task in report["tasks"]:
        client_short_keys.update(task["clients"].values())
        summary["tasks"].append({
            "task_id": task["task_id"], "clients": task["clients"],
            "workspaces": [step.get("evidence", {}) for step in task["steps"] if step["id"] in ["create", "create_workspace", "audio_create_workspace"]],
        })
        if task["status"] != "passed":
            errors.append(f"{name}/{task['task_id']}: {task['status']}")
        for step in task.get("cleanup", []):
            if step["status"] != "passed":
                errors.append(f"{name}/{task['task_id']}: cleanup {step['status']}")
    summary["reports"].append({
        "name": name, "tasks": len(report["tasks"]), "status": report["status"],
        "cleanup_steps": sum(len(t.get("cleanup", [])) for t in report["tasks"]),
        "sha256": hashlib.sha256((reports / name).read_bytes()).hexdigest(),
    })

ended = datetime.datetime.fromisoformat((reports / "after-all.time").read_text().strip().replace("Z", "+00:00"))
for manifest_path in sorted((reports / "profiles").rglob("manifest.json")):
    manifest = json.loads(manifest_path.read_text())
    for item in manifest["profiles"]:
        content = (manifest_path.parent / item["name"]).read_bytes()
        if len(content) != item["size"] or hashlib.sha256(content).hexdigest() != item["sha256"]:
            errors.append(f"profile manifest mismatch: {manifest_path}/{item['name']}")
    profile = manifest_path.parent / "goroutine.pprof"
    trace = subprocess.check_output(["go", "tool", "pprof", "-traces", str(profile)], text=True, stderr=subprocess.DEVNULL)
    profile.with_suffix(".txt").write_text(trace)
    blocks = trace.split("-----------")
    counts = {}
    counts["goroutines"] = sum(int(match.group(1)) for block in blocks if (match := re.search(r"^\s*(\d+)\s+runtime\.", block)))
    for key, pattern in [("observer_wait", "WaitForObservers"), ("execute_wait", "eino.(*turnRun).execute")]:
        counts[key] = sum(int(re.search(r"^\s*(\d+)", block).group(1)) for block in blocks if pattern in block)
    summary["profiles"].append({
        "captured_at": manifest["captured_at"], "sequence": manifest["sequence"],
        "path": str(profile.relative_to(reports)), **counts,
    })

delayed = [p for p in summary["profiles"] if datetime.datetime.fromisoformat(p["captured_at"].replace("Z", "+00:00")) >= ended + datetime.timedelta(seconds=10)]
if len(delayed) < 2:
    errors.append("missing delayed profiling snapshots")
for profile in delayed:
    if profile["observer_wait"] or profile["execute_wait"]:
        errors.append(f"retained Eino waiters: {profile}")

# Join short report identities to full persisted Peer keys without exposing
# credentials. Keep the full mapping only in the private, ignored evidence.
db_path = run / "runtime/server/data/business.sqlite"
db = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
full_keys = {}
alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"


def short_key(key):
    value = 0
    for character in key:
        value = value * 58 + alphabet.index(character)
    decoded = bytes(len(key) - len(key.lstrip("1"))) + value.to_bytes((value.bit_length() + 7) // 8, "big")
    if len(decoded) != 32:
        raise ValueError("invalid persisted Giznet public key")
    return decoded[:4].hex()


for (key,) in db.execute("SELECT public_key FROM peer_runs"):
    short = short_key(key)
    if short in client_short_keys:
        if short in full_keys and full_keys[short] != key:
            errors.append("ambiguous short Peer identity")
        full_keys[short] = key
for key in client_short_keys:
    if len(key) > 8:
        full_keys[short_key(key)] = key
if any(key not in full_keys and key not in full_keys.values() for key in client_short_keys):
    errors.append("incomplete report-to-Peer identity mapping")
owners = set(full_keys.values())
workspaces = [dict(zip(["id", "owner", "name", "pending_deletion_id"], row)) for row in db.execute("SELECT id, owner_public_key, name, pending_deletion_id FROM workspaces") if row[1] in owners]
pending = [dict(zip(["resource_id", "owner", "status", "phase"], row)) for row in db.execute("SELECT resource_id, owner_public_key, task_status, task_phase FROM workspace_pending_deletions") if row[1] in owners]
states = [row for row in db.execute("SELECT owner_id, workspace_id FROM graph_states") if row[0] in owners]
scopes = [row for row in db.execute("SELECT owner_id, workspace_id, retired FROM graph_state_scopes") if row[0] in owners]
summary["ownership"] = {
    "checked": "local SQLite owner_public_key/owner_id; graph retirement markers",
    "peer_keys": full_keys, "remaining_workspaces": workspaces,
    "pending_workspace_deletions": pending, "remaining_graph_states": states,
    "graph_scope_markers": scopes,
    "not_verified": ["Mem0 scopes", "PostgreSQL", "Redis", "all filesystem objects", "physical Peer-run row reclamation"],
}
if workspaces or pending or states or any(not row[2] for row in scopes):
    errors.append("local Workspace cleanup has not reached a terminal state")
db.close()
summary["errors"] = errors
(reports / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
if errors:
    raise SystemExit("Observer lifecycle qualification failed:\n- " + "\n- ".join(errors))
print(f"Observer lifecycle qualified: {sum(r['tasks'] for r in summary['reports'])} tasks; delayed Eino waits=0")
