#!/usr/bin/env bash
# Isolated quota acceptance with a real GizClaw Server and provider HTTP fixtures.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../../.." && pwd)"
cd "$repo_dir"
mkdir -p tests/gizclaw-e2e/.testbench
run_dir="$(mktemp -d "$repo_dir/tests/gizclaw-e2e/.testbench/quota-XXXXXX")"
export GIZCLAW_QUOTA_STATE="$run_dir/runtime"
export GIZCLAW_QUOTA_REPORTS="$run_dir/reports"
mkdir -p "$GIZCLAW_QUOTA_STATE" "$GIZCLAW_QUOTA_REPORTS"
project="gizclaw-quota-test-$$"
export GIZCLAW_QUOTA_IMAGE="${GIZCLAW_QUOTA_IMAGE:-$project}"
compose=(docker compose -p "$project" -f tests/gizclaw-e2e/docker/compose.quota.yaml)
cleanup() {
  "${compose[@]}" logs --no-color > "$GIZCLAW_QUOTA_REPORTS/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  echo "Quota E2E reports: $GIZCLAW_QUOTA_REPORTS"
}
trap cleanup EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
if ! docker image inspect "$base" >/dev/null 2>&1; then
  docker build -f build/gizclaw/Dockerfile.cn.base -t "$base" build
fi
if [[ "${GIZCLAW_QUOTA_SKIP_HOST_BUILD:-}" != "1" ]]; then
  npm ci
  npm run build:console
fi
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/giztest"
docker run --rm --entrypoint /bin/bash -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
  -v "${GIZCLAW_QUOTA_MODCACHE:-gizclaw-monitor-modcache}:/gomod" \
  -v "${GIZCLAW_QUOTA_BUILDCACHE:-gizclaw-monitor-buildcache}:/cache" \
  -e GOMODCACHE=/gomod -e GOCACHE=/cache "$base" -lc \
  'cd /src && go build -o /out/gizclaw ./cmd/gizclaw && go build -o /out/quota-fixture ./tests/gizclaw-e2e/cmd/quota-fixture'
cp -R tests/gizclaw-e2e/docker/monitor "$image_dir/tests/gizclaw-e2e/docker/"
cp -R tests/gizclaw-e2e/giztest/quota "$image_dir/tests/gizclaw-e2e/giztest/"
docker build --build-arg "BASE_IMAGE=$base" -f tests/gizclaw-e2e/docker/Dockerfile.monitor -t "$GIZCLAW_QUOTA_IMAGE" "$image_dir"
docker run --rm -v "$GIZCLAW_QUOTA_STATE:/state" --entrypoint quota-fixture "$GIZCLAW_QUOTA_IMAGE" -init /state
"${compose[@]}" up -d --wait fixture server edge
"${compose[@]}" run --rm seed
"${compose[@]}" run --rm test
