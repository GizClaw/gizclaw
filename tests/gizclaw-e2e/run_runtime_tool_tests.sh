#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
if [[ -z "${GIZCLAW_RUNTIME_TOOL_PROVIDER_KEY:-}" ]]; then
  exec python3 "$script_dir/testdata/runtime-tools/load_credential.py" "$0" "$@"
fi
cd "$repo_dir"
export GIZCLAW_RUNTIME_TOOL_PARALLEL="${GIZCLAW_RUNTIME_TOOL_PARALLEL:-3}"
mkdir -p "$repo_dir/.testbench"
run_dir="$(mktemp -d "$repo_dir/.testbench/runtime-tools-XXXXXXXX")"
state_dir="$(mktemp -d /tmp/gizclaw-runtime-tools-state-XXXXXXXX)"
export GIZCLAW_MONITOR_STATE="$state_dir"
export GIZCLAW_MONITOR_REPORTS="$run_dir/reports"
export GIZCLAW_MONITOR_IMAGE="gizclaw-runtime-tools-$$"
mkdir -p "$run_dir/image/bin" "$GIZCLAW_MONITOR_REPORTS"
project="gizclaw-runtime-tools-$$"
run_user="$(id -u):$(id -g)"
cat > "$run_dir/compose.runtime-tools.yaml" <<'COMPOSE'
services:
  server:
    user: "${RUNTIME_TOOLS_RUN_USER}"
    logging: {driver: json-file, options: {max-size: "256m", max-file: "1"}}
  edge:
    user: "${RUNTIME_TOOLS_RUN_USER}"
    logging: {driver: json-file, options: {max-size: "256m", max-file: "1"}}
  toolcontrol:
    logging: {driver: json-file, options: {max-size: "32m", max-file: "1"}}
    image: ${GIZCLAW_MONITOR_IMAGE}
    user: "${RUNTIME_TOOLS_RUN_USER}"
    env_file: ["${GIZCLAW_MONITOR_STATE}/runtime.env"]
    environment: [GIZCLAW_RUNTIME_TOOL_PROVIDER_KEY]
    entrypoint: [monitor-seed]
    command: [-server, "server:9820", -profile-id, runtime-tools, -token-id, runtime-tools, -token, runtime-tools, -runtime-tools, -control-listen, ":9822", -timeout, 3h]
    depends_on: {server: {condition: service_healthy}}
    healthcheck:
      test: [CMD, curl, -fsS, "http://127.0.0.1:9822/ready"]
      interval: 1s
      timeout: 3s
      retries: 90
  test:
    user: "${RUNTIME_TOOLS_RUN_USER}"
    environment:
      GIZCLAW_TEST_ENDPOINT: "${GIZCLAW_RUNTIME_TOOL_TEST_ENDPOINT:-edge:9821}"
      GIZCLAW_TEST_REGISTRATION_TOKEN: runtime-tools
      GIZCLAW_RUNTIME_TOOL_PARALLEL: "${GIZCLAW_RUNTIME_TOOL_PARALLEL}"
COMPOSE
export RUNTIME_TOOLS_RUN_USER="$run_user"
compose=(docker compose -p "$project" -f "$script_dir/docker/compose.monitor.yaml" -f "$run_dir/compose.runtime-tools.yaml")
cleanup() {
  status=$?
  "${compose[@]}" logs --no-color > "$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  if [[ -d "$state_dir/server/data/objects/runtime-tool-profiling" ]]; then
    if ! cp -R "$state_dir/server/data/objects/runtime-tool-profiling" "$GIZCLAW_MONITOR_REPORTS/profiles"; then
      printf 'Runtime Tool profiling evidence copy failed.\n' >&2
      status=1
    fi
  fi
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$GIZCLAW_MONITOR_IMAGE" >/dev/null 2>&1 || true
  rm -rf "$state_dir" "$run_dir/image"
  printf 'Runtime Tool Docker E2E exit=%s reports=%s\n' "$status" "$GIZCLAW_MONITOR_REPORTS"
  exit "$status"
}
trap cleanup EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64|aarch64) arch=arm64;; esac
base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
if ! docker image inspect "$base" >/dev/null 2>&1; then
  docker build -f "$repo_dir/build/gizclaw/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
fi
if [[ ! -d node_modules ]]; then npm ci; fi
npm run build:console
image_dir="$run_dir/image"
python3 "$script_dir/testdata/runtime-tools/source_receipt.py" "$repo_dir" "$GIZCLAW_MONITOR_REPORTS/source-before.json"
docker run --rm --entrypoint /bin/bash \
  -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
  -v "$(go env GOMODCACHE):/root/go/pkg/mod" \
  -v "gizclaw-runtime-tools-buildcache:/root/.cache/go-build" "$base" -lc 'cd /src \
    && CGO_ENABLED=1 GOMAXPROCS=2 GOGC=25 GOMEMLIMIT=768MiB go build -p 2 -ldflags="-s -w" -o /out/gizclaw ./cmd/gizclaw \
    && CGO_ENABLED=1 GOMAXPROCS=2 GOGC=25 GOMEMLIMIT=768MiB go build -p 2 -ldflags="-s -w" -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
    && CGO_ENABLED=1 GOMAXPROCS=2 GOGC=25 GOMEMLIMIT=768MiB go build -p 2 -ldflags="-s -w" -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture'
python3 "$script_dir/testdata/runtime-tools/source_receipt.py" "$repo_dir" "$GIZCLAW_MONITOR_REPORTS/source-after.json"
if ! cmp -s "$GIZCLAW_MONITOR_REPORTS/source-before.json" "$GIZCLAW_MONITOR_REPORTS/source-after.json"; then
 printf 'Runtime sources changed during Docker build; this attempt cannot qualify.\n' >&2
 exit 1
fi
mkdir -p "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/testdata"
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp -R "$script_dir/testdata/runtime-tools" "$image_dir/tests/gizclaw-e2e/testdata/"
if [[ "${GIZCLAW_RUNTIME_TOOL_SMOKE_ONLY:-0}" != 1 ]]; then
 python3 "$script_dir/testdata/runtime-tools/generate.py" "$image_dir/tests/gizclaw-e2e/testdata/runtime-tools/giztest" --filter "${GIZCLAW_RUNTIME_TOOL_CASE_FILTER:-}" --repeat "${GIZCLAW_RUNTIME_TOOL_REPEAT:-3}"
fi

mkdir -p "$GIZCLAW_MONITOR_REPORTS/inputs"
cp "$image_dir/tests/gizclaw-e2e/testdata/runtime-tools/giztest/"*.giztest.yaml "$GIZCLAW_MONITOR_REPORTS/inputs/"
cp "$image_dir/tests/gizclaw-e2e/testdata/runtime-tools/workflow.json" "$GIZCLAW_MONITOR_REPORTS/inputs/"
if [[ -f "$image_dir/tests/gizclaw-e2e/testdata/runtime-tools/giztest/manifest.json" ]]; then
 cp "$image_dir/tests/gizclaw-e2e/testdata/runtime-tools/giztest/manifest.json" "$GIZCLAW_MONITOR_REPORTS/inputs/"
fi
shasum -a 256 "$image_dir/bin/"* > "$GIZCLAW_MONITOR_REPORTS/binaries.sha256"
git rev-parse HEAD > "$GIZCLAW_MONITOR_REPORTS/base-commit.txt"
git diff --binary HEAD > "$GIZCLAW_MONITOR_REPORTS/source.patch"

docker build -f "$script_dir/docker/Dockerfile.audioplayer" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm --user "$run_user" -v "$state_dir:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
python3 - "$state_dir/server/config.yaml" <<'PYPROFILE'
from pathlib import Path
import sys
path = Path(sys.argv[1])
config = path.read_text()
if config.count("\nservices:\n") != 1:
    raise SystemExit("Runtime Tool fixture requires one services section")
config = config.replace("\nservices:\n", "\n  runtime-tool-profiling:\n    kind: objectstore\n    storage: local-files\n    prefix: runtime-tool-profiling\nservices:\n")
config += "\nprofiling:\n  enabled: true\n  store: runtime-tool-profiling\n"
path.write_text(config)
PYPROFILE
touch "$state_dir/fixture.env"
"${compose[@]}" up -d --wait server edge toolcontrol
"${compose[@]}" run --rm --entrypoint /bin/bash test -lc 'gizclaw test validate -f /src/tests/gizclaw-e2e/testdata/runtime-tools/giztest'
set +e
"${compose[@]}" run --rm --entrypoint /bin/bash test -lc 'gizclaw test run --parallel "$GIZCLAW_RUNTIME_TOOL_PARALLEL" --evidence full --output /reports/giztest.json /src/tests/gizclaw-e2e/testdata/runtime-tools/giztest/*.giztest.yaml'
run_status=$?
python3 "$script_dir/testdata/runtime-tools/report.py" "$GIZCLAW_MONITOR_REPORTS"
score_status=$?
set -e
if [[ "$run_status" != 0 || "$score_status" != 0 ]]; then exit 1; fi
