#!/usr/bin/env bash
set -euo pipefail

if (($# != 1)); then
	echo "usage: $0 <10|20>" >&2
	exit 2
fi

concurrency="$1"
case "$concurrency" in
10)
	test_timeout="90m"
	profiling_enabled=0
	;;
20)
	test_timeout="180m"
	profiling_enabled=1
	;;
*)
	echo "unsupported workflow concurrency gate: $concurrency" >&2
	exit 2
	;;
esac
setup_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script_dir="$(cd "$setup_dir/.." && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
env_file="$script_dir/.env"
artifact_dir="${GIZCLAW_E2E_WORKFLOW_CONCURRENCY_ARTIFACT_DIR:-$script_dir/testdata/workflow-concurrency}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-workflow-concurrency-${concurrency}.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
export GIZCLAW_E2E_WORKFLOW_CONCURRENCY_ARTIFACT_DIR="$artifact_dir"
export GIZCLAW_E2E_PROFILING="$profiling_enabled"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-wc${concurrency}-${project_user:-user}-$$"
stats_output="$artifact_dir/container-stats.ndjson"
stats_pid=""
stack_started=0
profiles_collected=0

# shellcheck source=credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

compose_args() {
	printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE"
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY"
	fi
}

stop_resource_sampler() {
	if [[ -z "$stats_pid" ]]; then
		return
	fi
	kill "$stats_pid" >/dev/null 2>&1 || true
	wait "$stats_pid" >/dev/null 2>&1 || true
	stats_pid=""
}

collect_failure_logs() {
	if [[ "$stack_started" != "1" ]]; then
		return
	fi
	local -a args=()
	while IFS= read -r arg; do
		args+=("$arg")
	done < <(compose_args)
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" ps --all >&2 || true
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" logs --no-color --tail=200 server edge edge2 2>&1 |
		python3 "$setup_dir/redact_diagnostics.py" >&2 || true
}

collect_runtime_profiles() {
	if [[ "$profiling_enabled" != "1" || "$profiles_collected" == "1" || "$stack_started" != "1" ]]; then
		return
	fi
	local -a args=()
	local container_id output
	while IFS= read -r arg; do
		args+=("$arg")
	done < <(compose_args)
	container_id="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" ps -q server)"
	if [[ -z "$container_id" ]]; then
		echo "profiling enabled but the server container is unavailable" >&2
		return 1
	fi
	output="$artifact_dir/profiling/$GIZCLAW_E2E_DOCKER_PROJECT"
	mkdir -p "$output"
	if ! docker cp "$container_id:/src/tests/gizclaw-e2e/testdata/server-workspace/data/profiling/pprof/." "$output/"; then
		echo "failed to collect runtime profiles from server container $container_id" >&2
		return 1
	fi
	if [[ -z "$(find "$output" -type f -name manifest.json -size +0c -print -quit)" ]]; then
		echo "profiling enabled but no complete runtime profile manifest was collected" >&2
		return 1
	fi
	profiles_collected=1
}

cleanup() {
	local status=$?
	stop_resource_sampler
	if ((status != 0)); then
		collect_failure_logs
	fi
	if ! collect_runtime_profiles && ((status == 0)); then
		status=1
	fi
	if [[ "$stack_started" == "1" || -f "$docker_env_path" ]]; then
		if ! bash "$setup_dir/docker-compose-down.sh" && ((status == 0)); then
			status=1
		fi
	fi
	rm -f "$docker_env_path"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "==> build host e2e CLI"
mkdir -p "$script_dir/testdata/bin" "$artifact_dir"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)

echo "==> start isolated Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT"
bash "$setup_dir/docker-compose-up.sh"
stack_started=1
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

start_resource_sampler() {
	local -a args=()
	while IFS= read -r arg; do
		args+=("$arg")
	done < <(compose_args)
	: >"$stats_output"
	(
		while true; do
			container_ids=()
			while IFS= read -r container_id; do
				if [[ -n "$container_id" ]]; then
					container_ids+=("$container_id")
				fi
			done < <(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" ps -q server edge edge2 2>/dev/null || true)
			if ((${#container_ids[@]} > 0)); then
				captured_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
				while IFS= read -r sample; do
					printf '{"captured_at":"%s","sample":%s}\n' "$captured_at" "$sample"
				done < <(docker stats --no-stream --format '{{json .}}' "${container_ids[@]}" 2>/dev/null || true)
			fi
			sleep 2
		done
	) >>"$stats_output" &
	stats_pid=$!
}

start_resource_sampler
echo "==> run fixed Workflow concurrency=$concurrency selection"
workflow_cases=(
	"benchmark.doubao-realtime-conversation.concurrency-${concurrency}.giztest.yaml"
	"benchmark.doubao-realtime-conversation.concurrency-interrupt-${concurrency}.giztest.yaml"
	"benchmark.doubao-realtime-duplex-conversation.concurrency-${concurrency}.giztest.yaml"
	"benchmark.doubao-realtime-duplex-conversation.concurrency-interrupt-${concurrency}.giztest.yaml"
	"benchmark.flowcraft-voice-assistant.concurrency-${concurrency}.giztest.yaml"
	"benchmark.flowcraft-voice-assistant.concurrency-interrupt-${concurrency}.giztest.yaml"
	"benchmark.eino-concurrency-assistant.concurrency-${concurrency}.giztest.yaml"
	"benchmark.eino-concurrency-assistant.concurrency-interrupt-${concurrency}.giztest.yaml"
	"benchmark.volc-ast-translate.concurrency-${concurrency}.giztest.yaml"
	"benchmark.volc-ast-translate.concurrency-interrupt-${concurrency}.giztest.yaml"
)
workflow_paths=()
for workflow_case in "${workflow_cases[@]}"; do
	workflow_paths+=("$script_dir/giztest/$workflow_case")
done
report_path="$artifact_dir/giztest-concurrency-${concurrency}.json"
(cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run \
	--parallel "$concurrency" \
	--output "$report_path" \
	"${workflow_paths[@]}")
python3 - "$report_path" "$concurrency" <<'PY'
import json
import sys

path, concurrency = sys.argv[1], int(sys.argv[2])
with open(path, encoding="utf-8") as handle:
    report = json.load(handle)
expected = 10 * concurrency
tasks = report.get("tasks", [])
if report.get("status") != "passed" or len(tasks) != expected:
    raise SystemExit(
        f"invalid Giztest concurrency report: status={report.get('status')} "
        f"tasks={len(tasks)} expected={expected}"
    )
PY

stop_resource_sampler
if [[ ! -s "$stats_output" ]]; then
	echo "workflow concurrency resource sampler produced no data" >&2
	exit 1
fi
collect_runtime_profiles
echo "==> tear down isolated Docker e2e stack"
bash "$setup_dir/docker-compose-down.sh"
stack_started=0

if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT")" ]]; then
	echo "Docker containers remain after workflow concurrency cleanup" >&2
	exit 1
fi
if [[ -d "$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT" ]]; then
	echo "Docker state directory remains after workflow concurrency cleanup" >&2
	exit 1
fi

rm -f "$docker_env_path"
trap - EXIT INT TERM
echo "==> Workflow concurrency=$concurrency e2e completed artifacts=$artifact_dir"
