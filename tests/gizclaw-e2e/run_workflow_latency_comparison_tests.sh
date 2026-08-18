#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
gizclaw_binary="$script_dir/testdata/bin/gizclaw"
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

mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$gizclaw_binary" ./cmd/gizclaw)
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout 45m \
	-run '^TestWorkflowDriverVoiceLatencyComparison$' ./tests/gizclaw-e2e/go/chat)
