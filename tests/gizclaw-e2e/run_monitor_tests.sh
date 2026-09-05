#!/usr/bin/env bash
# Isolated, model-free Monitor API acceptance through giztest and real stores.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/monitor-XXXXXX")"
export GIZCLAW_MONITOR_STATE="$run_dir/runtime"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
mkdir -p "$GIZCLAW_MONITOR_STATE" "$GIZCLAW_MONITOR_REPORTS"
project="gizclaw-monitor-test-$$"
export GIZCLAW_MONITOR_IMAGE="${GIZCLAW_MONITOR_IMAGE:-$project}"
compose=(docker compose -p "$project" -f "$script_dir/docker/compose.monitor.yaml")
cleanup() {
  "${compose[@]}" logs --no-color > "$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$run_dir/image"
  echo "Monitor E2E reports: $GIZCLAW_MONITOR_REPORTS"
}
trap cleanup EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
if ! docker image inspect "$base" >/dev/null 2>&1; then
  docker build -f "$repo_dir/build/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
fi
# Build the browser assets on the host and native Linux binaries with persistent
# caches. Only binaries, contracts and fixtures enter the runtime image.
npm ci
npm run build:monitor
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/testdata/audio" "$image_dir/tests/gizclaw-e2e/giztest"
docker run --rm --entrypoint /bin/bash \
  -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
  -v "${GIZCLAW_MONITOR_MODCACHE:-gizclaw-monitor-modcache}:/gomod" \
  -v "${GIZCLAW_MONITOR_BUILDCACHE:-gizclaw-monitor-buildcache}:/cache" \
  -e GOMODCACHE=/gomod -e GOCACHE=/cache "$base" -lc 'cd /src \
    && go build -o /out/gizclaw ./cmd/gizclaw \
    && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
    && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture'
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp "$script_dir"/giztest/server.monitor.*.giztest.yaml "$image_dir/tests/gizclaw-e2e/giztest/"
cp "$script_dir/testdata/audio/sfu-tone.ogg" "$image_dir/tests/gizclaw-e2e/testdata/audio/"
docker build --build-arg "BASE_IMAGE=$base" -f "$script_dir/docker/Dockerfile.monitor" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
touch "$GIZCLAW_MONITOR_STATE/fixture.env"
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed
"${compose[@]}" run --rm fixture > "$run_dir/fixture.json"
python3 - "$run_dir/fixture.json" "$GIZCLAW_MONITOR_STATE/fixture.env" <<'PY'
import json, pathlib, sys
values = json.loads(pathlib.Path(sys.argv[1]).read_text())
pathlib.Path(sys.argv[2]).write_text("".join(f"{key}={value}\n" for key, value in values.items()))
PY
"${compose[@]}" run --rm test
