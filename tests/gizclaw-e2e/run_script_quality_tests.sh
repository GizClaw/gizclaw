#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
setup_dir="$script_dir/setup"
repo_root="$(cd "$script_dir/../.." && pwd)"
env_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
artifact_dir="${GIZCLAW_E2E_SCRIPT_QUALITY_ARTIFACT_DIR:-$script_dir/testdata/script-quality}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-script-quality.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-quality-${project_user:-user}-$$"
stack_started=0

cases="${GIZCLAW_E2E_SCRIPT_QUALITY_CASES:-werewolf murder-mystery poetry journey storyteller}"
read -r -a selected_cases <<< "$cases"
for name in "${selected_cases[@]}"; do
    case "$name" in
        werewolf|murder-mystery|poetry|journey|storyteller) ;;
        *) echo "unknown screenplay quality case: $name" >&2; exit 2 ;;
    esac
done

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

# Called transitively by the EXIT trap.
# shellcheck disable=SC2329
compose_args() {
	printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE"
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY"
	fi
}

# Called transitively by the EXIT trap.
# shellcheck disable=SC2329
collect_failure_logs() {
	if [[ "$stack_started" != "1" ]]; then
		return
	fi
	local -a args=()
	while IFS= read -r arg; do
		args+=("$arg")
	done < <(compose_args)
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" ps --all >&2 || true
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${args[@]}" logs --no-color --tail=3000 server edge edge2 2>&1 |
		python3 "$setup_dir/redact_diagnostics.py" > "$artifact_dir/bootstrap-failure.log" || true
    echo "bootstrap diagnostics: $artifact_dir/bootstrap-failure.log" >&2
}

# EXIT/INT/TERM own this cleanup path.
# shellcheck disable=SC2329
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
(cd "$repo_root" && npm ci && npm run build:console)
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)

echo "==> start isolated Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT"
bash "$setup_dir/docker-compose-up.sh"
stack_started=1
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

# Preserve native graphs/rules/memory while grading text independently of TTS.
quality_resources="$artifact_dir/text-workflows.json"
(cd "$repo_root" && go run ./tests/gizclaw-e2e/internal/scriptqualityresources --output "$quality_resources")
server_container="$(docker ps -q --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" --filter label=com.docker.compose.service=server)"
docker cp "$quality_resources" "$server_container:/tmp/gizclaw-script-quality-workflows.json"
docker exec "$server_container" sh -c 'XDG_CONFIG_HOME=/src/tests/gizclaw-e2e/testdata/cmd-config-home /src/tests/gizclaw-e2e/testdata/bin/gizclaw admin apply --context admin -f /tmp/gizclaw-script-quality-workflows.json'

# Semantic failures are retained as evidence; every selected scenario runs.
# The CLI's full report is explicit because judge explanations quote dialogue.
status=0
for name in "${selected_cases[@]}"; do
    report="$artifact_dir/$name.json"
    echo "==> screenplay quality: $name"
    if ! "$script_dir/testdata/bin/gizclaw" test run --parallel 1 --evidence full \
        --output "$report" "$script_dir/giztest/script-quality.$name.giztest.yaml"; then
        status=1
    fi
    python3 "$script_dir/setup/script_quality_report.py" "$report" || status=1
done
exit "$status"
