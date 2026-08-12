#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact="${GIZCLAW_E2E_GATEWAY_NATIVE_CHANNELS_ARTIFACT:-$script_dir/testdata/gateway-native-channels-2048.json}"

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "2,048-session native-channel qualification requires a clean repository HEAD" >&2
  exit 2
fi
qualification_head="$(git -C "$repo_root" rev-parse HEAD)"

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

cleanup() {
  bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> build clean-head capacity binaries"
mkdir -p "$script_dir/testdata/bin"
bash "$setup_dir/build-linux-cgo.sh"
export GIZCLAW_E2E_GATEWAY_LINUX_PREBUILT=1
export GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE="gizclaw-native-channels:head-${qualification_head:0:16}"

echo "==> start one-Edge, one-upstream native-channel stack"
bash "$setup_dir/docker-compose-up.sh" --gateway-native-channels-2048
set -a
# shellcheck disable=SC1091
source "$script_dir/testdata/docker/current.env"
set +a

echo "==> establish 2,048 sessions, complete exact-byte service traffic, and hold 8,192 channels"
(cd "$repo_root" && go run ./tests/gizclaw-e2e/gateway-capacity \
  -edges "$GIZCLAW_E2E_EDGE_ENDPOINT" \
  -signaling-base-from-edge \
  -sessions 2048 \
  -ramp 5m \
  -duration 0s \
  -ping-interval 30s \
  -dial-timeout 20s \
  -ping-timeout 10s \
  -speed-bytes 1024 \
  -speed-baseline-bytes 1024 \
  -speed-timeout 5m \
  -concurrency 512 \
  -max-establishment-failures 0 \
  -max-ping-failures 0 \
  -required-upstreams-per-edge 1 \
  -max-upstreams-per-edge 1 \
  -max-sessions-per-upstream 2048 \
  -hold-service-id 1 \
  -cleanup-timeout 2m \
  -artifact "$artifact")

edge_id="$(docker ps -q \
  --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" \
  --filter "label=com.docker.compose.service=edge")"
if [[ -z "$edge_id" ]]; then
  echo "native-channel qualification Edge container is missing" >&2
  exit 1
fi
usage_log="$(docker exec "$edge_id" sh -c \
  "grep 'gateway tunnel channel usage' /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log" || true)"
observed_peak="$(sed -n 's/.*peak_active_channels=\([0-9][0-9]*\).*/\1/p' <<<"$usage_log" | sort -n | tail -1)"
observed_after_cleanup="$(sed -n 's/.* active_channels=\([0-9][0-9]*\) .*/\1/p' <<<"$usage_log" | tail -1)"
if [[ -z "$observed_peak" || -z "$observed_after_cleanup" ]]; then
  echo "Edge did not emit native-channel usage evidence" >&2
  exit 1
fi
artifact_tmp="$(mktemp "${artifact}.tmp.XXXXXX")"
jq \
  --argjson peak "$observed_peak" \
  --argjson after "$observed_after_cleanup" \
  '.native_channels.observed = true |
   .native_channels.observed_peak_active = $peak |
   .native_channels.observed_after_cleanup = $after' \
  "$artifact" >"$artifact_tmp"
mv "$artifact_tmp" "$artifact"

jq -e '
  .established == 2048 and
  .native_channels.persistent == 6144 and
  .native_channels.held_services == 2048 and
  .native_channels.peak_active == 8192 and
  .native_channels.expected_after_cleanup == 0 and
  .native_channels.observed == true and
  .native_channels.observed_peak_active == 8192 and
  .native_channels.observed_after_cleanup == 0 and
  .identity_crossover == false and
  .unexpected_disconnects == 0 and
  .speed_test.upload.concurrent.completed == 2048 and
  .speed_test.download.concurrent.completed == 2048 and
  .cleanup.serve_completed == true and
  .cleanup.timed_out == false and
  .passed == true
' "$artifact" >/dev/null

current_head="$(git -C "$repo_root" rev-parse HEAD)"
if [[ "$current_head" != "$qualification_head" || -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "qualification head changed: start=$qualification_head current=$current_head" >&2
  exit 1
fi

echo "==> native-channel qualification artifact: $artifact"
