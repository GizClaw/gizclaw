#!/usr/bin/env bash
set -euo pipefail

# Compares the Eino audio-input path with the ASR path on the same synthesized
# recordings in several languages and dialects. Set GIZCLAW_AUDIO_SAMPLES to a
# comma-separated sample id list to run a subset.

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
gizclaw_binary="$script_dir/testdata/bin/gizclaw"
artifact_dir="${GIZCLAW_E2E_AUDIO_INPUT_ARTIFACT_DIR:-$script_dir/testdata/audio-input-comparison}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-audio-input.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-audio-input-${project_user:-user}-$$"
stack_started=0

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

collect_failure_logs() {
	if [[ "$stack_started" != "1" ]]; then
		return
	fi
	local compose_file="${GIZCLAW_E2E_DOCKER_COMPOSE_FILE:-$script_dir/docker/docker-compose.yaml}"
	local -a compose_args=(-f "$compose_file")
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		compose_args+=(-f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")
	fi
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_args[@]}" logs \
		--no-color --tail=200 server 2>&1 |
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
	rm -f "$docker_env_path" "$gizclaw_binary"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

rm -rf "$artifact_dir/runs" "$artifact_dir/report.json" "$artifact_dir/report.md"
mkdir -p "$script_dir/testdata/bin" "$artifact_dir/runs"

echo "==> build audio input comparison CLI"
(cd "$repo_root" && npm ci && npm run build:console)
(cd "$repo_root" && go build -o "$gizclaw_binary" ./cmd/gizclaw)

echo "==> start isolated Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT"
bash "$setup_dir/docker-compose-up.sh"
stack_started=1
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

python3 "$setup_dir/audio_input_comparison.py" \
	"$gizclaw_binary" \
	"$script_dir/giztest/benchmark.eino-audio-input-comparison.giztest.yaml" \
	"$artifact_dir"

echo "==> audio input comparison report=$artifact_dir/report.md"
