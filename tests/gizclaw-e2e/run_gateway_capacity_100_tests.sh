#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
gateway_upstream_path="${GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH:-relay}"
case "$gateway_upstream_path" in
  direct | relay) ;;
  *) echo "GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH must be direct or relay" >&2; exit 2 ;;
esac
export GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="$gateway_upstream_path"

export GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR="${GIZCLAW_E2E_GATEWAY_100_ARTIFACT_DIR:-$script_dir/testdata/gateway-capacity-extended/$gateway_upstream_path/sessions-100-burst}"
gateway_gomaxprocs="$(getconf _NPROCESSORS_ONLN)"
export GIZCLAW_E2E_GATEWAY_GOMAXPROCS="$gateway_gomaxprocs"
export GIZCLAW_E2E_GATEWAY_GOGC=100
export GIZCLAW_E2E_GATEWAY_DIAL_TIMEOUT=20s
export GIZCLAW_E2E_GATEWAY_PING_TIMEOUT=28s
export GIZCLAW_E2E_GATEWAY_SPEED_BYTES=1048576
export GIZCLAW_E2E_GATEWAY_SPEED_BASELINE_BYTES=33554432
export GIZCLAW_E2E_GATEWAY_SPEED_TIMEOUT=2m
export GIZCLAW_E2E_GATEWAY_MIN_SPEED_AGGREGATE_RATIO=0
export GIZCLAW_E2E_GATEWAY_MIN_UPLOAD_AGGREGATE_MBPS=200
export GIZCLAW_E2E_GATEWAY_MIN_DOWNLOAD_AGGREGATE_MBPS=200
export GIZCLAW_E2E_GATEWAY_MIN_ESTABLISHMENT_RATE=20
export GIZCLAW_E2E_GATEWAY_MAX_DIAL_P95=1s
export GIZCLAW_E2E_GATEWAY_MAX_DIAL_P99=5s
export GIZCLAW_E2E_GATEWAY_CONCURRENCY=100
export GIZCLAW_E2E_GATEWAY_REQUIRED_UPSTREAMS_PER_EDGE=4

# shellcheck source=run_gateway_extended_capacity_tests.sh
# shellcheck disable=SC1091
source "$script_dir/run_gateway_extended_capacity_tests.sh"

for repetition in 1 2 3; do
  run_case sessions-100-burst 100 0s 0s "$repetition" false
done

# Defined by the sourced extended-capacity runner.
# shellcheck disable=SC2154
echo "==> 100-session burst artifact set: $artifact_set"
