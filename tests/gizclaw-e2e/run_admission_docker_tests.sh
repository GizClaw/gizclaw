#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
cd "$repo_root"
for tool in docker python3 curl; do
  command -v "$tool" >/dev/null || { echo "required tool missing: $tool" >&2; exit 1; }
done
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-admission-docker.XXXXXX")"
GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-admission-$(date +%s)-$$"
export GIZCLAW_E2E_DOCKER_PROJECT
export GIZCLAW_E2E_DOCKER_ENV="$task_dir/docker.env"
stack_started=0
# shellcheck disable=SC2329 # Invoked by traps.
cleanup() {
  local status=$?
  trap - EXIT
  if [[ "$stack_started" == 1 ]]; then
    bash "$script_dir/setup/docker-compose-down.sh" || status=1
  fi
  rm -rf "$task_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy COMPOSE_PROFILES
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$script_dir/setup/build-admission-runners.sh"
go build -o "$task_dir/gizclaw" ./cmd/gizclaw
export GIZCLAW_ADMISSION_GO_RUNNER="$task_dir/gizclaw"
stack_started=1
bash "$script_dir/setup/docker-compose-up.sh" --admission
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a

# Edge terminates its own handshake. Bootstrap the configured Admin through
# that supported ingress, then use direct Server signaling for all test cases.
cp -R "$GIZCLAW_E2E_CONFIG_HOME/gizclaw/admin" "$GIZCLAW_E2E_CONFIG_HOME/gizclaw/admission-bootstrap"
GIZCLAW_BOOTSTRAP_ENDPOINT="$GIZCLAW_E2E_EDGE_ENDPOINT" \
  perl -0pi -e 's/^(\s*endpoint:\s*)[^\s]+/${1}$ENV{GIZCLAW_BOOTSTRAP_ENDPOINT}/mg' \
  "$GIZCLAW_E2E_CONFIG_HOME/gizclaw/admission-bootstrap/config.yaml"
export XDG_CONFIG_HOME="$GIZCLAW_E2E_CONFIG_HOME"
"$task_dir/gizclaw" connect set-name "Admission Admin" --context admission-bootstrap >/dev/null

go test -tags=gizclaw_e2e ./tests/gizclaw-e2e/go/admissiondocker -count=1 -v
