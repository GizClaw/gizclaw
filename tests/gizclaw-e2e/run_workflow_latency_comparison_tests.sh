#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
gizclaw_binary="$script_dir/testdata/bin/gizclaw"
artifact_dir="${GIZCLAW_E2E_WORKFLOW_LATENCY_ARTIFACT_DIR:-$script_dir/testdata/workflow-latency}"
# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-workflow-latency.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"

cleanup() {
	if [[ -f "$docker_env_path" ]]; then
		bash "$setup_dir/docker-compose-down.sh" || true
	fi
	rm -f "$docker_env_path"
	rm -f "$gizclaw_binary"
}
trap cleanup EXIT INT TERM

mkdir -p "$script_dir/testdata/bin" "$artifact_dir/runs"
(cd "$repo_root" && npm ci && npm run build:console)
(cd "$repo_root" && go build -o "$gizclaw_binary" ./cmd/gizclaw)
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

eino_planner="$script_dir/giztest/benchmark.eino-planner-latency-comparison.voice-latency.giztest.yaml"
eino_baseline="$script_dir/giztest/benchmark.eino-latency-comparison.voice-latency.giztest.yaml"

echo "==> warm up latency workflows"
"$gizclaw_binary" test run --parallel 1 "$eino_planner"
"$gizclaw_binary" test run --parallel 1 "$eino_baseline"

reports=()
for pair in 1 2 3 4 5; do
	if ((pair % 2 == 1)); then
		order=(eino_planner eino_baseline)
	else
		order=(eino_baseline eino_planner)
	fi
	for driver in "${order[@]}"; do
		case "$driver" in
		eino_planner) test_file="$eino_planner" ;;
		eino_baseline) test_file="$eino_baseline" ;;
		esac
		report="$artifact_dir/runs/pair-${pair}-${driver}.json"
		"$gizclaw_binary" test run --parallel 1 --output "$report" "$test_file"
		reports+=("$report")
	done
done

python3 - "$artifact_dir/report.json" "${reports[@]}" <<'PY'
import json
import statistics
import sys

output, *paths = sys.argv[1:]
metrics = ("first_text_ms", "first_audio_ms", "text_eos_ms", "audio_eos_ms")
samples = {name: {key: [] for key in metrics} for name in ("eino_planner", "eino_baseline")}
for path in paths:
    driver = next((name for name in samples if path.endswith(f"-{name}.json")), None)
    if driver is None:
        raise SystemExit(f"unknown latency scenario report: {path}")
    with open(path, encoding="utf-8") as handle:
        report = json.load(handle)
    if report.get("status") != "passed" or len(report.get("tasks", [])) != 1:
        raise SystemExit(f"invalid latency run report: {path}")
    steps = report["tasks"][0].get("steps", [])
    stream = next((step for step in steps if step.get("operation") == "peer_stream"), None)
    if not stream:
        raise SystemExit(f"missing peer_stream evidence: {path}")
    evidence = stream.get("evidence", {})
    for metric in metrics:
        value = evidence.get(metric)
        if not isinstance(value, (int, float)):
            raise SystemExit(f"missing {metric} evidence: {path}")
        samples[driver][metric].append(value)
summary = {
    driver: {
        metric: {
            "samples_ms": values,
            "mean_ms": statistics.fmean(values),
            "median_ms": statistics.median(values),
        }
        for metric, values in driver_samples.items()
    }
    for driver, driver_samples in samples.items()
}
with open(output, "w", encoding="utf-8") as handle:
    json.dump({"version": "gizclaw.workflow-latency/v1", "pairs": 5, "summary": summary}, handle, indent=2)
    handle.write("\n")
PY

echo "==> Workflow latency comparison completed report=$artifact_dir/report.json"
