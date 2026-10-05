"""Score actual native Giztest receipts; missing work never counts as PASS."""
import argparse
from collections import Counter, defaultdict
from datetime import datetime
import html
import json
from pathlib import Path


def load_document(path):
    text = path.read_text()
    return json.loads(text[text.index("{"):])


def resolve(value, variables):
    if isinstance(value, str) and value.startswith("${") and value.endswith("}"):
        return variables.get(value[2:-1], value)
    if isinstance(value, dict):
        return {key: resolve(item, variables) for key, item in value.items()}
    if isinstance(value, list):
        return [resolve(item, variables) for item in value]
    return value


def expected_requests(document, variables):
    groups = {}
    turn = None
    for step in document["steps"]:
        if step["id"].startswith("turn_"):
            turn = step["id"]
        operation = step.get("client_rpc", {})
        if operation.get("method") != "client.mhs.v0.write" and operation.get("tool") not in {
            "audioplayer.play", "audioplayer.stop", "audioplayer.mode.set", "run.workspace.set"
        }:
            continue
        expected = step.get("expect", {})
        count = expected.get("/requests", {}).get("count")
        if count is None:
            continue
        rows = [{} for _ in range(count)]
        for pointer, rule in expected.items():
            if not pointer.startswith("/requests/") or "equals" not in rule:
                continue
            parts = pointer.split("/")
            rows[int(parts[2])][parts[3]] = resolve(rule["equals"], variables)
        key = operation.get("tool", operation["method"])
        previous = groups.get(key, [])
        for index, row in enumerate(rows):
            row["turn_id"] = previous[index]["turn_id"] if index < len(previous) else turn
        groups[key] = rows
    return [row for rows in groups.values() for row in rows]


def percentile(values, percent):
    if not values:
        return None
    ordered = sorted(values)
    return ordered[min(len(ordered) - 1, int((len(ordered) - 1) * percent))]


def request_identity(row, part):
    procedure = row.get("tool", "client.mhs.v0.write")
    arguments = row.get("args", {})
    if part == "target":
        target = {key: row[key] for key in ("id", "hwd", "hwd_enum", "tool_enum") if key in row}
        if procedure == "run.workspace.set":
            target["workspace_name"] = arguments.get("workspace_name")
        return row.get("turn_id"), procedure, target
    if "effective_index" in row:
        return row.get("turn_id"), procedure, {"index": row["effective_index"]}
    parameters = {key: value for key, value in arguments.items() if procedure != "run.workspace.set" or key != "workspace_name"}
    return row.get("turn_id"), procedure, parameters


def matching_parts(expected, actual, part):
    available = [request_identity(row, part) for row in actual]
    matched = 0
    for row in expected:
        identity = request_identity(row, part)
        if identity in available:
            available.remove(identity)
            matched += 1
    return matched


def task_metrics(task, document):
    variables = {}
    steps = task.get("steps", [])
    for step in steps:
        if step["id"].startswith("create_"):
            workspace = step.get("evidence", {}).get("workspace", {})
            variables[step["id"].removeprefix("create_")] = workspace.get("name")
    expected = expected_requests(document, variables)
    actual_groups = {}
    for step in steps + task.get("cleanup", []):
        evidence = step.get("evidence", {})
        if step["id"].startswith("audit_") and "requests" in evidence:
            actual_groups[step["id"]] = evidence["requests"]
    actual = [row for rows in actual_groups.values() for row in rows]
    turns = [step for step in steps if step["id"].startswith("turn_") and step.get("evidence", {}).get("user_input") is not None]
    turn_starts = [(datetime.fromisoformat(step["started_at"].replace("Z", "+00:00")).timestamp() * 1000, step["id"]) for step in turns]
    for row in actual:
        starts = [(start, identifier) for start, identifier in turn_starts if start <= row.get("received_at_unix_ms", -1)]
        row["turn_id"] = max(starts)[1] if starts else None
    unmatched = list(actual)
    matched = []
    for wanted in expected:
        found = next((row for row in unmatched if all(row.get(key) == value for key, value in wanted.items())), None)
        if found is not None:
            unmatched.remove(found)
            matched.append(found)
    ready_ms = []
    for request in matched:
        received = request.get("received_at_unix_ms")
        if received is None:
            continue
        prior = [datetime.fromisoformat(step["started_at"].replace("Z", "+00:00")).timestamp() * 1000 for step in turns]
        prior = [start for start in prior if start <= received]
        if prior:
            ready_ms.append(round(received - max(prior), 3))
    return {"expected_actions": len(expected), "observed_actions": len(actual), "correct_actions": len(matched),
            "parameter_correct_actions": matching_parts(expected, actual, "parameter"),
            "target_correct_actions": matching_parts(expected, actual, "target"),
            "extra_or_wrong_actions": len(unmatched), "planned_turns": sum(step["id"].startswith("turn_") for step in document["steps"]),
            "observed_turns": len(turns), "protocol_ready_ms": ready_ms, "trace_present": bool(actual_groups)}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("reports", type=Path)
    args = parser.parse_args()
    report = json.loads((args.reports / "giztest.json").read_text())
    documents = {path.name: load_document(path) for path in (args.reports / "inputs").glob("*.giztest.yaml")}
    expected_count = sum(document.get("repeat", 1) for document in documents.values())
    counts = Counter(task["status"] for task in report["tasks"])
    metrics = defaultdict(list)
    rows = []
    for task in report["tasks"]:
        document = documents.get(Path(task["path"]).name)
        if document is None:
            continue
        details = task_metrics(task, document)
        # Static smoke documents have no audit receipts; do not invent action metrics.
        if details["trace_present"] and details["planned_turns"]:
            metrics[task["name"].split(".")[-1]].append(details)
        rows.append({"name": task["name"], "status": task["status"], "repeat": task.get("repeat_index"),
                     "duration_ms": task["duration_ms"], "error": task.get("error", ""), **details})
    groups = {}
    for count, samples in metrics.items():
        expected = sum(row["expected_actions"] for row in samples)
        observed = sum(row["observed_actions"] for row in samples)
        correct = sum(row["correct_actions"] for row in samples)
        parameters = sum(row["parameter_correct_actions"] for row in samples)
        targets = sum(row["target_correct_actions"] for row in samples)
        times = [value for row in samples for value in row["protocol_ready_ms"]]
        groups[count] = {"samples": len(samples), "expected_actions": expected, "observed_actions": observed,
                         "correct_actions": correct, "action_recall": correct / expected if expected else None,
                         "mutating_call_precision": correct / observed if observed else None,
                         "parameter_correct_actions": parameters, "parameter_accuracy": parameters / observed if observed else None,
                         "target_correct_actions": targets, "target_accuracy": targets / observed if observed else None,
                         "protocol_ready_p50_ms": percentile(times, .5), "protocol_ready_p95_ms": percentile(times, .95)}
    repetitions = defaultdict(list)
    for row in rows:
        repetitions[row["name"]].append(row["status"])
    missing = expected_count - len(report["tasks"])
    passed = report["status"] == "passed" and missing == 0 and counts.get("skipped", 0) == 0 and len(rows) == expected_count
    summary = {"status": "PASS" if passed else "FAIL", "native_status": report["status"], "expected_tasks": expected_count,
               "actual_tasks": len(report["tasks"]), "missing_tasks": missing, "counts": dict(counts), "by_tool_count": groups,
               "repeat_stability": {name: {"attempts": len(statuses), "passed": statuses.count("passed"), "all_passed": all(value == "passed" for value in statuses)} for name, statuses in repetitions.items()},
               "notes": ["Scores use decoded device requests and exact native assertions, including failed attempts.",
                         "Action metrics count mutating requests; discovery and read-only calls are checked separately by native assertions.",
                         "Recall is fully correct actions divided by all planned expected actions, including later turns not reached after failure.",
                         "Call precision requires both exact target and parameters in the authorized user turn. Parameter and target accuracies independently match expected requests one-to-one within the same user turn and procedure.",
                         "A failed task can stop before later planned turns; those turns remain unobserved.",
                         "Protocol ready time measures user-turn start to receipt of the complete validated request at the device; it includes transport.",
                         "Catalog focus metadata is a test fixture update, not a production UI focus API.",
                         "Program and audio acknowledgments do not qualify physical hardware or actual audio playout."], "tasks": rows}
    (args.reports / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    table = "".join("<tr><td>" + "</td><td>".join(html.escape(str(row[key])) for key in ["name", "repeat", "status", "duration_ms", "correct_actions", "expected_actions", "observed_actions", "error"]) + "</td></tr>" for row in rows)
    page = "<!doctype html><meta charset=utf-8><title>Runtime Tool native acceptance</title><style>body{font:15px system-ui;margin:32px}table{border-collapse:collapse;width:100%}td,th{border:1px solid #ddd;padding:7px;text-align:left}input{padding:8px;width:320px}pre{white-space:pre-wrap}</style>"
    page += "<h1>Runtime Tool native acceptance — " + summary["status"] + "</h1><pre>" + html.escape(json.dumps({key: value for key, value in summary.items() if key not in {"tasks", "repeat_stability"}}, ensure_ascii=False, indent=2)) + "</pre>"
    page += "<input id=filter placeholder='Filter case or status'><table><thead><tr><th>Case</th><th>Repeat</th><th>Status</th><th>ms</th><th>Correct</th><th>Expected</th><th>Observed</th><th>Failure</th></tr></thead><tbody>" + table + "</tbody></table><script>document.querySelector('#filter').oninput=e=>document.querySelectorAll('tbody tr').forEach(row=>row.hidden=!row.textContent.toLowerCase().includes(e.target.value.toLowerCase()))</script>"
    (args.reports / "report.html").write_text(page)
    print(json.dumps({"status": summary["status"], "expected_tasks": expected_count, "counts": dict(counts)}, ensure_ascii=False))
    raise SystemExit(0 if passed else 1)


if __name__ == "__main__":
    main()
