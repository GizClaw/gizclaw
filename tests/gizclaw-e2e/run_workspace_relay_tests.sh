#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
setup_dir="$script_dir/setup"
repo_root="$(cd "$script_dir/../.." && pwd)"
env_file="$script_dir/.env"
artifact_dir="${GIZCLAW_E2E_WORKSPACE_RELAY_ARTIFACT_DIR:-$script_dir/testdata/workspace-relay}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-workspace-relay.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-relay-${project_user:-user}-$$"
stack_started=0

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

compose_args() {
	printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE"
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY"
	fi
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

cleanup() {
	local status=$?
	if ((status != 0)); then
		collect_failure_logs
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

verify_report() {
	python3 - "$1" "$2" <<'PY'
import json
import sys

path, expected = sys.argv[1], int(sys.argv[2])
with open(path, encoding="utf-8") as handle:
    report = json.load(handle)
tasks = report.get("tasks", [])
if report.get("status") != "passed" or len(tasks) != expected:
    raise SystemExit(
        f"invalid workspace-relay report: status={report.get('status')} "
        f"tasks={len(tasks)} expected={expected}"
    )
for task in tasks:
    relay_steps = [step for step in task.get("steps", []) if step.get("operation") == "workspace_relay"]
    if not relay_steps:
        raise SystemExit(f"task {task.get('task_id')} ran no workspace_relay step")
    for step in relay_steps:
        evidence = step.get("evidence") or {}
        if "terminal_client" not in evidence or "completed_turns" not in evidence:
            raise SystemExit(f"task {task.get('task_id')} relay evidence is incomplete: {evidence}")
PY
}

echo "==> run repeat-1 workspace relay gate"
single_report="$artifact_dir/workspace-relay-1.json"
(cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run \
	--parallel 1 \
	--output "$single_report" \
	"$script_dir/giztest/workspace-relay.workflow-tester.giztest.yaml")
verify_report "$single_report" 1

echo "==> run repeat-20 workspace relay gate"
benchmark_report="$artifact_dir/workspace-relay-20.json"
(cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run \
	--parallel 20 \
	--output "$benchmark_report" \
	"$script_dir/giztest/benchmark.workspace-relay.workflow-tester-20.giztest.yaml")
verify_report "$benchmark_report" 20

echo "==> workspace relay gates passed"
