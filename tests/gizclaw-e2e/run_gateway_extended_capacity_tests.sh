#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact_root="$script_dir/testdata/gateway-capacity-extended"
artifact_base="${GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR:-$artifact_root}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
current_env=""
runtime_state=""

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

mkdir -p "$artifact_root" "$script_dir/testdata/docker" "$script_dir/testdata/bin"
artifact_root="$(cd "$artifact_root" && pwd -P)"
if [[ -e "$artifact_base" ]]; then
  artifact_base="$(cd "$artifact_base" && pwd -P)"
else
  artifact_parent="$(cd "$(dirname "$artifact_base")" && pwd -P)"
  artifact_base="$artifact_parent/$(basename "$artifact_base")"
fi
case "$artifact_base/" in
  "$artifact_root/"*) ;;
  *)
    echo "extended capacity artifacts must stay below $artifact_root: $artifact_base" >&2
    exit 2
    ;;
esac
mkdir -p "$artifact_base"
artifact_base="$(cd "$artifact_base" && pwd -P)"
artifact_set="$artifact_base/$run_id"
runs_dir="$artifact_set/runs"
bin_dir="$artifact_set/bin"
gateway_bin="$bin_dir/gateway-capacity"
runtime_state="$(mktemp -d "$script_dir/testdata/docker/gateway-capacity.XXXXXX")"
mkdir -p "$runs_dir" "$bin_dir"

cleanup_current() {
  if [[ -z "$current_env" || ! -f "$current_env" ]]; then
    return 0
  fi
  GIZCLAW_E2E_DOCKER_ENV="$current_env" bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || return 1
  rm -f "$current_env"
  current_env=""
}

cleanup_on_exit() {
  local status="$?"
  if ! cleanup_current; then
    echo "failed to clean the active gateway-capacity Docker project; env=$current_env" >&2
    status=1
  fi
  rmdir "$runtime_state" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup_on_exit EXIT

read_gateway_limit() {
  local key="$1"
  awk -v key="$key:" '$1 == key { print $2; found = 1; exit } END { if (!found) exit 1 }' \
    "$script_dir/testdata/edge-workspace/config.yaml.template"
}

max_sessions_per_edge="$(read_gateway_limit max-sessions)"
max_upstreams_per_edge="$(read_gateway_limit max-upstreams)"
max_sessions_per_upstream="$(read_gateway_limit sessions-per-upstream)"
if [[ "$max_sessions_per_edge" != "30000" || "$max_upstreams_per_edge" != "16" || "$max_sessions_per_upstream" != "2048" ]]; then
  echo "extended capacity requires Edge limits 30000/16/2048; configured $max_sessions_per_edge/$max_upstreams_per_edge/$max_sessions_per_upstream" >&2
  exit 2
fi

echo "==> build host e2e CLI and extended gateway-capacity runner"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
(cd "$repo_root" && go build -o "$gateway_bin" ./tests/giznet-e2e/gateway)

run_case() {
  local scenario="$1"
  local sessions="$2"
  local ramp="$3"
  local hold="$4"
  local repetition="$5"
  local soak="$6"
  local project_slug artifact
  project_slug="$(printf '%s-%s-%s' "$run_id" "$scenario" "$repetition" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
  artifact="$runs_dir/${scenario}-run-${repetition}.json"
  current_env="$runtime_state/${scenario}-run-${repetition}.env"
  echo "==> start fresh capacity stack: scenario=$scenario repetition=$repetition sessions=$sessions"
  GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-capacity-$project_slug" \
    GIZCLAW_E2E_DOCKER_ENV="$current_env" \
    bash "$setup_dir/docker-compose-up.sh" --gateway-capacity

  set -a
  # shellcheck disable=SC1090
  source "$current_env"
  set +a

  echo "==> run extended capacity workload: scenario=$scenario repetition=$repetition"
  (cd "$repo_root" && GOMAXPROCS=8 "$gateway_bin" \
    -edges "$GIZCLAW_E2E_EDGE_ENDPOINT,$GIZCLAW_E2E_EDGE2_ENDPOINT" \
    -sessions "$sessions" \
    -ramp "$ramp" \
    -duration "$hold" \
    -ping-interval 30s \
    -dial-timeout 20s \
    -ping-timeout 10s \
    -speed-bytes 0 \
    -concurrency 512 \
    -max-establishment-failures 0 \
    -max-ping-failures 0 \
    -max-ping-round-duration 30s \
    -require-balanced-edges \
    -max-sessions-per-edge "$max_sessions_per_edge" \
    -required-upstreams-per-edge "$max_upstreams_per_edge" \
    -max-upstreams-per-edge "$max_upstreams_per_edge" \
    -max-sessions-per-upstream "$max_sessions_per_upstream" \
    -require-role-resources \
    -docker-project "$GIZCLAW_E2E_DOCKER_PROJECT" \
    -docker-compose-file "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE" \
    -scenario "$scenario" \
    -repetition "$repetition" \
    -soak="$soak" \
    -artifact "$artifact")

  echo "==> tear down fresh capacity stack: scenario=$scenario repetition=$repetition"
  cleanup_current
}

for repetition in 1 2 3; do
  run_case sessions-100 100 30s 5m "$repetition" false
done
for repetition in 1 2 3; do
  run_case sessions-500 500 2m30s 5m "$repetition" false
done
for repetition in 1 2 3; do
  run_case sessions-1000 1000 5m 5m "$repetition" false
done
run_case soak-1000 1000 5m 60m 1 true

echo "==> analyze extended capacity artifacts"
(cd "$repo_root" && "$gateway_bin" \
  -analyze-dir "$runs_dir" \
  -artifact "$artifact_set/projection.json")

echo "==> extended gateway-capacity artifact set: $artifact_set"
