#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact="${GIZCLAW_E2E_GATEWAY_CAPACITY_ARTIFACT:-$script_dir/testdata/gateway-capacity-100.json}"
# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

cleanup() {
  bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> build Linux CGO e2e CLI"
mkdir -p "$script_dir/testdata/bin"
bash "$setup_dir/build-linux-cgo.sh"
export GIZCLAW_E2E_GATEWAY_LINUX_PREBUILT=1

echo "==> start Docker e2e stack"
bash "$setup_dir/docker-compose-up.sh" --gateway-capacity
set -a
# shellcheck disable=SC1091
source "$script_dir/testdata/docker/current.env"
set +a

echo "==> run 100-session multi-Edge capacity and concurrent throughput check"
(cd "$repo_root" && go run ./tests/gizclaw-e2e/gateway-capacity \
  -edges "$GIZCLAW_E2E_EDGE_ENDPOINT,$GIZCLAW_E2E_EDGE2_ENDPOINT" \
  -sessions 100 \
  -ramp 30s \
  -duration 1m \
  -ping-interval 10s \
  -dial-timeout 20s \
  -ping-timeout 10s \
  -speed-bytes 4194304 \
  -speed-baseline-bytes 33554432 \
  -speed-timeout 2m \
  -min-speed-aggregate-ratio 0.8 \
  -min-upload-aggregate-mbps 200 \
  -min-download-aggregate-mbps 200 \
  -concurrency 100 \
  -artifact "$artifact")

echo "==> 100-session capacity artifact: $artifact"
