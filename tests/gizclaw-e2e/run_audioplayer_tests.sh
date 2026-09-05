#!/usr/bin/env bash
# Model-free audioplayer Giztests against an isolated real Server and Edge.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/audioplayer-XXXXXX")"
# Reuse the monitor stack's ephemeral identities, SQLite runtime and seed.
export GIZCLAW_MONITOR_STATE="$run_dir/runtime"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
mkdir -p "$GIZCLAW_MONITOR_STATE" "$GIZCLAW_MONITOR_REPORTS"
project="gizclaw-audioplayer-test-$$"
export GIZCLAW_MONITOR_IMAGE="$project"
compose=(docker compose -p "$project" -f "$script_dir/docker/compose.monitor.yaml")
cleanup() {
  "${compose[@]}" logs --no-color > "$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$GIZCLAW_MONITOR_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$run_dir/image"
  echo "Audioplayer E2E reports: $GIZCLAW_MONITOR_REPORTS"
}
trap cleanup EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
npm ci
npm run build:monitor
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/giztest"
if [[ "$(go env GOOS)/$(go env GOARCH)" == "linux/$arch" ]]; then
  go build -o "$image_dir/bin/gizclaw" ./cmd/gizclaw
  go build -o "$image_dir/bin/monitor-seed" ./tests/gizclaw-e2e/cmd/multiserver-seed
  go build -o "$image_dir/bin/monitor-fixture" ./tests/gizclaw-e2e/cmd/monitor-fixture
else
  base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
  if ! docker image inspect "$base" >/dev/null 2>&1; then
    docker build -f "$repo_dir/build/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
  fi
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
    -v "$(go env GOMODCACHE):/root/go/pkg/mod" \
    -v "gizclaw-audioplayer-buildcache:/root/.cache/go-build" "$base" -lc 'cd /src \
      && go build -o /out/gizclaw ./cmd/gizclaw \
      && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
      && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture'
fi
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp "$script_dir"/giztest/server.device.audioplayer.*.giztest.yaml "$image_dir/tests/gizclaw-e2e/giztest/"
docker build -f "$script_dir/docker/Dockerfile.audioplayer" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
touch "$GIZCLAW_MONITOR_STATE/fixture.env"
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed
"${compose[@]}" run --rm test \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.control.giztest.yaml \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.clear.giztest.yaml \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.invalid.giztest.yaml \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.rejected.giztest.yaml \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.unsupported.giztest.yaml \
  /src/tests/gizclaw-e2e/giztest/server.device.audioplayer.telemetry.giztest.yaml \
  --output /reports/giztest.json
