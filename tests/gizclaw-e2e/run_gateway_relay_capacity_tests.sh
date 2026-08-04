#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
artifact_root="$script_dir/testdata/gateway-capacity-extended"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
matrix_root="$artifact_root/relay-comparison-$run_id"
bin_dir="$matrix_root/bin"
gateway_bin="$bin_dir/gateway-capacity"

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "gateway relay capacity requires a clean repository HEAD" >&2
  exit 2
fi
repository_head="$(git -C "$repo_root" rev-parse HEAD)"
mkdir -p "$script_dir/testdata/bin" "$bin_dir"

echo "==> build capacity binaries once: head=$repository_head"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
(cd "$repo_root" && go build -o "$gateway_bin" ./tests/gizclaw-e2e/gateway-capacity)

export GIZCLAW_E2E_GATEWAY_PREBUILT=1
export GIZCLAW_E2E_GATEWAY_CAPACITY_BIN="$gateway_bin"

run_profile() {
  local path="$1"
  local sessions="$2"
  local target="$matrix_root/$path/sessions-$sessions"
  mkdir -p "$target"
  echo "==> capacity matrix profile: path=$path sessions=$sessions repetitions=3"
  if [[ "$sessions" == "100" ]]; then
    GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="$path" \
      GIZCLAW_E2E_GATEWAY_100_ARTIFACT_DIR="$target" \
      bash "$script_dir/run_gateway_capacity_100_tests.sh"
  else
    GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="$path" \
      GIZCLAW_E2E_GATEWAY_500_ARTIFACT_DIR="$target" \
      bash "$script_dir/run_gateway_capacity_500_tests.sh"
  fi
}

run_profile direct 100
run_profile relay 100
run_profile direct 500
run_profile relay 500

echo "==> run bounded pure-Giznet direct/Coturn causal diagnostic"
GIZNET_COTURN_ARTIFACT="$matrix_root/giznet-coturn.json" \
  bash "$repo_root/tests/giznet-e2e/run_coturn_tests.sh"

echo "==> compare exact 12-run direct/relay matrix"
"$gateway_bin" \
  -compare-relay-dir "$matrix_root" \
  -artifact "$matrix_root/comparison.json"

echo "==> gateway relay capacity passed: head=$repository_head artifact=$matrix_root/comparison.json"
