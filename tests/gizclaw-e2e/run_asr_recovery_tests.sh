#!/usr/bin/env bash
# Fault-injected SDK/ASR recovery through isolated Docker Server and Giztest.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/asr-recovery-XXXXXX")"
export GIZCLAW_MONITOR_STATE="$run_dir/runtime"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
mkdir -p "$GIZCLAW_MONITOR_STATE" "$GIZCLAW_MONITOR_REPORTS"
project="gizclaw-asr-recovery-test-$$"
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
export GIZCLAW_MONITOR_IMAGE="$base"
run_user="$(id -u):$(id -g)"
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/testdata"
git lfs pull --include="third_party/audio/prebuilt/*/linux-$arch/**,tests/gizclaw-e2e/testdata/audio/sfu-tone.ogg"

# Use the same local source-built base for compilation and runtime. Absolute
# entrypoints and read-only mounts always select this checkout's new binaries.
cat > "$run_dir/compose.user.yaml" <<EOF
x-asr-runtime: &asr-runtime
  image: "$base"
  working_dir: /src
  volumes:
    - "$image_dir/bin:/binaries:ro"
    - "$image_dir/tests:/src/tests:ro"
services:
  log-sink:
    <<: *asr-runtime
    entrypoint: [/binaries/log-sink]
  server:
    <<: *asr-runtime
    user: "$run_user"
    entrypoint: [/binaries/gizclaw, serve, --force, /workspace]
    environment: {LATENCY_TTS_STARTUP: 3s, LATENCY_TTS_SYNTHESIS: 0ms}
  edge:
    <<: *asr-runtime
    user: "$run_user"
    entrypoint: [/binaries/gizclaw, edge, serve, /workspace]
  seed:
    <<: *asr-runtime
    user: "$run_user"
    entrypoint: [/binaries/monitor-seed]
  test:
    <<: *asr-runtime
    user: "$run_user"
    entrypoint: [/binaries/gizclaw, test, run]
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
  docker rm -f "$project-build" >/dev/null 2>&1 || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$image_dir"
  echo "ASR recovery E2E reports: $GIZCLAW_MONITOR_REPORTS"
}
trap cleanup EXIT
if ! docker image inspect "$base" >/dev/null 2>&1; then
  docker build -f "$repo_dir/build/gizclaw/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
fi
npm ci && npm run build:console
python3 "$script_dir/testdata/asr-recovery/overlay.py" "$repo_dir" "$image_dir/bin" docker test
docker run --rm --name "$project-build" --cpus=4 --memory=6g -e GOMAXPROCS=4 -e GOFLAGS=-p=4 --entrypoint /bin/bash \
  -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
  -v "$(go env GOMODCACHE):/root/go/pkg/mod" \
  -v "gizclaw-asr-recovery-buildcache:/root/.cache/go-build" "$base" -lc 'cd /src \
    && go test -overlay /out/overlay.json ./pkgs/gizclaw/services/ai/peergenx -run TestASRRecoveryFixtureProtocol -count=1 \
    && go build -overlay /out/overlay.json -o /out/gizclaw ./cmd/gizclaw \
    && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
    && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture \
    && go build -o /out/log-sink ./tests/gizclaw-e2e/cmd/log-sink'
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp -R "$script_dir/testdata/asr-recovery" "$image_dir/tests/gizclaw-e2e/testdata/"
cp "$script_dir/testdata/audio/sfu-tone.ogg" "$image_dir/tests/gizclaw-e2e/testdata/"
python3 - "$repo_dir" "$GIZCLAW_MONITOR_REPORTS/manifest.json" "$image_dir/bin/gizclaw" <<'PYMANIFEST'
import hashlib
import json
import os
import pathlib
import subprocess
import sys
root, output, binary = map(pathlib.Path, sys.argv[1:])
paths = sorted((root / "pkgs/genx/transformers/doubaoasr").glob("*.go"))
paths += [root / "go.mod", root / "go.sum", root / "tests/gizclaw-e2e/run_asr_recovery_tests.sh"]
paths += [root / "tests/gizclaw-e2e/cmd/multiserver-seed/rtp_bos.go", root / "tests/gizclaw-e2e/testdata/slow-tts/provider.go"]
paths += sorted((root / "tests/gizclaw-e2e/testdata/asr-recovery").glob("*"))
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "dirty": bool(subprocess.check_output(["git", "-C", str(root), "status", "--porcelain"], text=True).strip()),
    "base_image": os.environ["GIZCLAW_MONITOR_IMAGE"],
    "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
    "failure_code": 55000000,
    "failure_turn": 8,
    "sha256": {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths if p.is_file()},
}, indent=2) + "\n")
PYMANIFEST
docker run --rm --user "$run_user" --workdir /src \
  -v "$image_dir/bin:/binaries:ro" -v "$image_dir/tests:/src/tests:ro" \
  -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint /binaries/monitor-fixture "$base" -init /state
python3 - "$GIZCLAW_MONITOR_STATE/server/config.yaml" <<'PYCONFIG'
import pathlib
import sys
path = pathlib.Path(sys.argv[1])
text = path.read_text()
assert text.count("level: info") == 1
path.write_text(text.replace("level: info", "level: warn"))
PYCONFIG
touch "$GIZCLAW_MONITOR_STATE/fixture.env"
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed -server server:9820 -profile-id asr-recovery -token-id asr-recovery -token monitor-test -asr-recovery
"${compose[@]}" run --rm test \
  /src/tests/gizclaw-e2e/testdata/asr-recovery/audio.giztest.yaml \
  --report /reports/giztest.json
"${compose[@]}" logs --no-color server > "$GIZCLAW_MONITOR_REPORTS/server.log"
python3 "$script_dir/testdata/asr-recovery/check_logs.py" "$GIZCLAW_MONITOR_REPORTS/server.log"
