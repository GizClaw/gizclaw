#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR="${GIZCLAW_E2E_GATEWAY_1000_SOAK_ARTIFACT_DIR:-$script_dir/testdata/gateway-capacity-extended/relay/sessions-1000-soak}"

# The sourced entrypoint initializes the exact relay-only topology and fixed
# 1,000-session settings without executing its main block.
# shellcheck source=run_gateway_capacity_1000_tests.sh
# shellcheck disable=SC1091
source "$script_dir/run_gateway_capacity_1000_tests.sh"

# Defined by the sourced 1,000-session entrypoint.
# shellcheck disable=SC2154
echo "==> qualify the exact clean head before the soak: $capacity_repository_head"
run_1000_burst_repetitions
verify_capacity_head_unchanged

echo "==> run the fixed 60-minute 1,000-session soak"
# Read by run_case from the sourced shared runner.
# shellcheck disable=SC2034
gateway_min_final_speed_retention=0.8
run_case sessions-1000-soak 1000 0s 60m 1 true

verify_capacity_head_unchanged
# Defined by the sourced shared runner.
# shellcheck disable=SC2154
echo "==> 1,000-session burst and soak artifact set: $artifact_set"
