#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact_dir="${GIZCLAW_E2E_WORKFLOW_CONCURRENCY_ARTIFACT_DIR:-$script_dir/testdata/workflow-concurrency}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-workflow-concurrency-10.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
export GIZCLAW_E2E_WORKFLOW_CONCURRENCY_ARTIFACT_DIR="$artifact_dir"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-wc10-${project_user:-user}-$$"
stats_pid=""
stack_started=0

# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

stop_resource_sampler() {
	if [[ -z "$stats_pid" ]]; then
		return
	fi
	kill "$stats_pid" >/dev/null 2>&1 || true
	wait "$stats_pid" >/dev/null 2>&1 || true
	stats_pid=""
}

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
	stop_resource_sampler
	if ((status != 0)); then
		collect_failure_logs
	fi
	if [[ "$stack_started" == "1" || -f "$docker_env_path" ]]; then
		bash "$setup_dir/docker-compose-down.sh" || status=$?
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
	local output="$artifact_dir/container-stats.ndjson"
	local -a args=()
	while IFS= read -r arg; do
		args+=("$arg")
	done < <(compose_args)
	(
		while true; do
			captured_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
			while IFS= read -r sample; do
				printf '{"captured_at":"%s","sample":%s}\n' "$captured_at" "$sample"
			done < <(
				docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" ps -q server edge edge2 2>/dev/null |
					xargs docker stats --no-stream --format '{{json .}}' 2>/dev/null || true
			)
			sleep 2
		done
	) >>"$output" &
	stats_pid=$!
}

start_resource_sampler
echo "==> run fixed Workflow concurrency=10 selection"
(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout 90m \
	-run '^Test(Realtime|RealtimeDuplex|Flowcraft|Eino|Translate)WorkflowConcurrency(Interrupt)?10$' \
	./tests/gizclaw-e2e/go/chat)

stop_resource_sampler
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
echo "==> Workflow concurrency=10 e2e completed artifacts=$artifact_dir"
