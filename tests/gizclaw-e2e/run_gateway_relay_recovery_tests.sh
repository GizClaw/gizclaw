#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
cleanup_done=0
project_suffix="$(printf '%s-%s' "${USER:-user}" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-gateway-relay-$project_suffix"
export GIZCLAW_E2E_DOCKER_ENV="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT.env"
export GIZCLAW_E2E_DOCKER_COMPOSE_FILE="$script_dir/docker/docker-compose.yaml"
export GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY="$script_dir/docker/docker-compose.gateway-relay.yaml"

# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
  if [[ "$cleanup_done" == "0" ]]; then
    bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
    rm -f "$GIZCLAW_E2E_DOCKER_ENV"
  fi
}
trap cleanup EXIT

mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
bash "$setup_dir/docker-compose-up.sh" --gateway-relay-recovery
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a

set +e
(cd "$repo_root" && go test -count=1 -v -tags gizclaw_e2e \
  -run '^TestGatewayRelayRecoversSameClientBeforeSessionAcceptance$' \
  ./tests/gizclaw-e2e/go/edge)
test_status=$?
set -e

project="$GIZCLAW_E2E_DOCKER_PROJECT"
state_dir="$script_dir/testdata/docker/$project"
bash "$setup_dir/docker-compose-down.sh"
rm -f "$GIZCLAW_E2E_DOCKER_ENV"
cleanup_done=1

remaining="$(docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.ID}}')"
if [[ -n "$remaining" ]]; then
  echo "gateway relay recovery teardown left project containers" >&2
  exit 1
fi
if [[ -e "$state_dir" ]]; then
  echo "gateway relay recovery teardown left project runtime state" >&2
  exit 1
fi
exit "$test_status"
