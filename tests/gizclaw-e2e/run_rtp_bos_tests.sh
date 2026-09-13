#!/usr/bin/env bash
# Model-free rtp-bos Giztests against an isolated real Server and Edge.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/rtp-bos-XXXXXX")"
# Reuse the monitor stack's ephemeral identities, SQLite runtime and seed.
export GIZCLAW_MONITOR_STATE="$run_dir/runtime"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
mkdir -p "$GIZCLAW_MONITOR_STATE" "$GIZCLAW_MONITOR_REPORTS"
python3 - "$repo_dir" "$GIZCLAW_MONITOR_REPORTS/manifest.json" <<'PYMANIFEST'
import hashlib
import json
import pathlib
import subprocess
import sys
root, output = map(pathlib.Path, sys.argv[1:])
paths = [root / "sdk/go/gizcli/peer_stream.go", root / "cmd/internal/commands/giztest/peer_stream.go"]
paths += [root / "tests/gizclaw-e2e/run_rtp_bos_tests.sh", root / "tests/gizclaw-e2e/cmd/multiserver-seed/rtp_bos.go"]
paths += sorted((root / "tests/gizclaw-e2e/testdata/rtp-bos").glob("*"))
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "tts_startup_ms": 3000,
    "tts_synthesis_ms": 0,
    "receiver_bos_delay_ms": 200,
    "sha256": {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths if p.is_file()},
}, indent=2) + "\n")
PYMANIFEST
project="gizclaw-rtp-bos-test-$$"
export GIZCLAW_MONITOR_IMAGE="$project"
# Match bind-mount ownership on Linux as well as Docker Desktop.
run_user="$(id -u):$(id -g)"
cat > "$run_dir/compose.user.yaml" <<EOF
services:
  server:
    user: "$run_user"
    environment: {LATENCY_TTS_STARTUP: 3s, LATENCY_TTS_SYNTHESIS: 0ms}
  edge: {user: "$run_user"}
  seed: {user: "$run_user"}
  test:
    user: "$run_user"
    environment:
      GIZCLAW_TEST_ENDPOINT: server:9820
      LATENCY_TONE: "$(base64 < "$script_dir/testdata/audio/sfu-tone.ogg" | tr -d '\n')"
networks:
  default:
    internal: true
EOF
compose=(docker compose -p "$project" -f "$script_dir/docker/compose.monitor.yaml" -f "$run_dir/compose.user.yaml")
cleanup() {
  "${compose[@]}" logs --no-color > "$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$GIZCLAW_MONITOR_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$run_dir/image"
  echo "RTP BOS E2E reports: $GIZCLAW_MONITOR_REPORTS"
}
trap cleanup EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
npm ci && npm run build:console
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker"
if [[ "$(go env GOOS)/$(go env GOARCH)" == "linux/$arch" ]]; then
  python3 "$script_dir/testdata/rtp-bos/overlay.py" "$repo_dir" "$image_dir"
  go build -overlay "$image_dir/overlay.json" -o "$image_dir/bin/gizclaw" ./cmd/gizclaw
  go build -o "$image_dir/bin/monitor-seed" ./tests/gizclaw-e2e/cmd/multiserver-seed
  go build -o "$image_dir/bin/monitor-fixture" ./tests/gizclaw-e2e/cmd/monitor-fixture
else
  base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
  if ! docker image inspect "$base" >/dev/null 2>&1; then
    docker build -f "$repo_dir/build/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
  fi
  python3 "$script_dir/testdata/rtp-bos/overlay.py" "$repo_dir" "$image_dir/bin" docker
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
    -v "$(go env GOMODCACHE):/root/go/pkg/mod" \
    -v "gizclaw-rtp-bos-buildcache:/root/.cache/go-build" "$base" -lc 'cd /src \
      && go build -overlay /out/overlay.json -o /out/gizclaw ./cmd/gizclaw \
      && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
      && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture'
fi
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
mkdir -p "$image_dir/tests/gizclaw-e2e/testdata"
cp -R "$script_dir/testdata/rtp-bos" "$image_dir/tests/gizclaw-e2e/testdata/"
cp -R "$script_dir/testdata/audio" "$image_dir/tests/gizclaw-e2e/testdata/"
docker build -f "$script_dir/docker/Dockerfile.audioplayer" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm --user "$run_user" -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
touch "$GIZCLAW_MONITOR_STATE/fixture.env"
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed -server server:9820 -profile-id rtp-bos -token-id rtp-bos -token monitor-test -rtp-bos
"${compose[@]}" run --rm test \
  /src/tests/gizclaw-e2e/testdata/rtp-bos/audio.giztest.yaml \
  --output /reports/giztest.json
