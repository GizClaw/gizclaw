#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"

if [[ "${GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH:-relay}" != "relay" ]]; then
  echo "1,000-session qualification requires relay-only Edge upstreams" >&2
  exit 2
fi
if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "1,000-session qualification requires a clean repository HEAD" >&2
  exit 2
fi
capacity_repository_head="$(git -C "$repo_root" rev-parse HEAD)"

export GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH=relay
export GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR="${GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR:-${GIZCLAW_E2E_GATEWAY_1000_ARTIFACT_DIR:-$script_dir/testdata/gateway-capacity-extended/relay/sessions-1000-burst}}"
gateway_gomaxprocs="$(getconf _NPROCESSORS_ONLN)"
export GIZCLAW_E2E_GATEWAY_GOMAXPROCS="$gateway_gomaxprocs"
export GIZCLAW_E2E_GATEWAY_GOGC=200
export GIZCLAW_E2E_GATEWAY_DIAL_TIMEOUT=20s
export GIZCLAW_E2E_GATEWAY_PING_TIMEOUT=28s
export GIZCLAW_E2E_GATEWAY_SPEED_BYTES=1048576
export GIZCLAW_E2E_GATEWAY_SPEED_BASELINE_BYTES=33554432
export GIZCLAW_E2E_GATEWAY_SPEED_TIMEOUT=2m
export GIZCLAW_E2E_GATEWAY_MIN_SPEED_AGGREGATE_RATIO=0
export GIZCLAW_E2E_GATEWAY_MIN_UPLOAD_AGGREGATE_MBPS=200
export GIZCLAW_E2E_GATEWAY_MIN_DOWNLOAD_AGGREGATE_MBPS=200
export GIZCLAW_E2E_GATEWAY_MIN_FINAL_SPEED_RETENTION=0
export GIZCLAW_E2E_GATEWAY_MIN_ESTABLISHMENT_RATE=20
export GIZCLAW_E2E_GATEWAY_MAX_DIAL_P95=1s
export GIZCLAW_E2E_GATEWAY_MAX_DIAL_P99=5s
export GIZCLAW_E2E_GATEWAY_CONCURRENCY=1000
export GIZCLAW_E2E_GATEWAY_REQUIRED_UPSTREAMS_PER_EDGE=4
export GIZCLAW_E2E_GATEWAY_CLEANUP_TIMEOUT=30s

# shellcheck source=run_gateway_extended_capacity_tests.sh
# shellcheck disable=SC1091
source "$script_dir/run_gateway_extended_capacity_tests.sh"

wait_capacity_stack_settle() {
  local remaining=120
  while ((remaining > 0)); do
    echo "==> capacity stack settle heartbeat: status=waiting remaining_seconds=$remaining"
    sleep 15
    remaining=$((remaining - 15))
  done
  echo "==> capacity stack settle heartbeat: status=ready remaining_seconds=0"
}

run_1000_burst_repetitions() {
  # Read by run_case from the sourced shared runner.
  # shellcheck disable=SC2034
  gateway_min_final_speed_retention=0
  for repetition in 1 2 3; do
    if ((repetition > 1)); then
      wait_capacity_stack_settle
    fi
    run_case sessions-1000-burst 1000 0s 30s "$repetition" false
  done
}

verify_capacity_head_unchanged() {
  local current_head
  current_head="$(git -C "$repo_root" rev-parse HEAD)"
  if [[ "$current_head" != "$capacity_repository_head" || -n "$(git -C "$repo_root" status --porcelain)" ]]; then
    echo "capacity evidence head changed during the run: start=$capacity_repository_head current=$current_head" >&2
    return 1
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  run_1000_burst_repetitions
  verify_capacity_head_unchanged
  # Defined by the sourced shared runner.
  # shellcheck disable=SC2154
  echo "==> 1,000-session burst artifact set: $artifact_set"
fi
