#!/usr/bin/env bash
# Provider-free observer lifecycle Giztests against a local Server and Edge.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/observer-lifecycle-XXXXXX")"
echo "Observer lifecycle reports: $run_dir/reports"
chmod 700 "$run_dir"
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
paths = [root / "pkgs/genx/agentkit/audiodock/dock.go", root / "cmd/internal/commands/giztest/peer_stream.go"]
paths += sorted((root / "tests/gizclaw-e2e/giztest").glob("slow-tts.*.giztest.yaml"))
paths += sorted((root / "tests/gizclaw-e2e/testdata/slow-tts").glob("*"))
paths += sorted((root / "tests/gizclaw-e2e/testdata/observer-lifecycle").glob("*"))
paths += [root / "pkgs/genx/internal/streamkit/output.go", root / "pkgs/genx/internal/streamkit/invocation.go", root / "pkgs/genx/transformers/eino/transformer.go"]
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "tts_startup_ms": 12000,
    "tts_synthesis_ms": 200,
    "sha256": {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths if p.is_file()},
}, indent=2) + "\n")
PYMANIFEST
project="gizclaw-observer-test-$$"
export GIZCLAW_MONITOR_IMAGE="$project"
# Match bind-mount ownership on Linux as well as Docker Desktop.
run_user="$(id -u):$(id -g)"
cat > "$run_dir/compose.user.yaml" <<EOF
services:
  server:
    user: "$run_user"
    environment: {LATENCY_TTS_STARTUP: 12s, LATENCY_TTS_SYNTHESIS: 200ms}
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
# shellcheck disable=SC2329
# Invoked by the EXIT trap.
cleanup() {
  "${compose[@]}" logs --no-color > "$GIZCLAW_MONITOR_REPORTS/containers.log" 2>&1 || true
  python3 - "$GIZCLAW_MONITOR_STATE/server/data/business.sqlite" "$GIZCLAW_MONITOR_REPORTS/ownership.sqlite" <<'PYBACKUP' || true
import pathlib, sqlite3, sys
source, target = map(pathlib.Path, sys.argv[1:])
if source.exists():
    with sqlite3.connect(f"file:{source}?mode=ro", uri=True) as src, sqlite3.connect(target) as dst:
        src.backup(dst)
PYBACKUP
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker image rm "$GIZCLAW_MONITOR_IMAGE" >/dev/null 2>&1 || true
  cp -R "$GIZCLAW_MONITOR_STATE/server/data/objects/profiling" "$GIZCLAW_MONITOR_REPORTS/profiles" 2>/dev/null || true
  rm -rf "$GIZCLAW_MONITOR_STATE" "$run_dir/image"
  echo "Observer lifecycle reports: $GIZCLAW_MONITOR_REPORTS"
}
trap 'cleanup' EXIT
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
npm ci && npm run build:console
image_dir="$run_dir/image"
mkdir -p "$image_dir/bin" "$image_dir/tests/gizclaw-e2e/docker" "$image_dir/tests/gizclaw-e2e/giztest"
if [[ "$(go env GOOS)/$(go env GOARCH)" == "linux/$arch" ]]; then
  python3 "$script_dir/testdata/slow-tts/overlay.py" "$repo_dir" "$image_dir"
  python3 "$script_dir/testdata/observer-lifecycle/overlay.py" "$repo_dir" "$image_dir"
  go build -overlay "$image_dir/overlay.json" -o "$image_dir/bin/gizclaw" ./cmd/gizclaw
  go build -overlay "$image_dir/overlay.json" -o "$image_dir/bin/monitor-seed" ./tests/gizclaw-e2e/cmd/multiserver-seed
  go build -o "$image_dir/bin/monitor-fixture" ./tests/gizclaw-e2e/cmd/monitor-fixture
else
  base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
  if ! docker image inspect "$base" >/dev/null 2>&1; then
    docker build -f "$repo_dir/build/gizclaw/Dockerfile.cn.base" -t "$base" "$repo_dir/build"
  fi
  python3 "$script_dir/testdata/slow-tts/overlay.py" "$repo_dir" "$image_dir/bin" docker
  python3 "$script_dir/testdata/observer-lifecycle/overlay.py" "$repo_dir" "$image_dir/bin" docker
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_dir:/src" -v "$image_dir/bin:/out" \
    -v "$(go env GOMODCACHE):/root/go/pkg/mod" \
    -v "gizclaw-observer-buildcache:/root/.cache/go-build" "$base" -lc 'cd /src \
      && go build -overlay /out/overlay.json -o /out/gizclaw ./cmd/gizclaw \
      && go build -overlay /out/overlay.json -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
      && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture'
fi
cp -R "$script_dir/docker/monitor" "$image_dir/tests/gizclaw-e2e/docker/"
cp "$script_dir"/giztest/slow-tts.*.giztest.yaml "$image_dir/tests/gizclaw-e2e/giztest/"
mkdir -p "$image_dir/tests/gizclaw-e2e/testdata"
cp -R "$script_dir/testdata/slow-tts" "$image_dir/tests/gizclaw-e2e/testdata/"
cp -R "$script_dir/testdata/audio" "$image_dir/tests/gizclaw-e2e/testdata/"
python3 - "$image_dir/tests/gizclaw-e2e/giztest" <<'PYHISTORY'
from pathlib import Path
import sys
for path in Path(sys.argv[1]).glob('slow-tts.eino.*.giztest.yaml'):
    text = path.read_text()
    step = "- id: delivered_history\n  client: audio_peer\n  rpc:\n    method: server.workspace.history.list\n    request:\n      workspace_name: ${audio_workspace_name}\n      order: WORKSPACE_HISTORY_LIST_REQUEST_ORDER_DESC\n      limit: 10\n  expect:\n    /available:\n      equals: true\n    /items/0/type:\n      equals: PEER_RUN_HISTORY_ENTRY_TYPE_AGENT\n    /items/0/text:\n      pattern: '^Lifecycle fixture reply'\n"
    path.write_text(text.replace('finally:\n', step + 'finally:\n'))
PYHISTORY
cp -R "$script_dir/testdata/observer-lifecycle" "$image_dir/tests/gizclaw-e2e/testdata/"
cp "$script_dir/testdata/observer-lifecycle/eino.json" "$image_dir/tests/gizclaw-e2e/testdata/slow-tts/eino.json"
docker build -f "$script_dir/docker/Dockerfile.audioplayer" -t "$GIZCLAW_MONITOR_IMAGE" "$image_dir"
docker run --rm --user "$run_user" -v "$GIZCLAW_MONITOR_STATE:/state" --entrypoint monitor-fixture "$GIZCLAW_MONITOR_IMAGE" -init /state
python3 - "$GIZCLAW_MONITOR_STATE/server/config.yaml" <<'PYPROFILE'
from pathlib import Path
import sys
p=Path(sys.argv[1]);s=p.read_text().replace('stores:\n','stores:\n  profiling:\n    kind: objectstore\n    storage: local-files\n    prefix: profiling\n',1)
p.write_text(s+'\nprofiling:\n  enabled: true\n  store: profiling\npending_deletion:\n  scan_interval: 1s\n')
PYPROFILE
touch "$GIZCLAW_MONITOR_STATE/fixture.env"
"${compose[@]}" up -d --wait server edge
"${compose[@]}" run --rm seed -server server:9820 -profile-id slow-tts -token-id slow-tts -token monitor-test -slow-tts
go version -m "$image_dir/bin/gizclaw" > "$GIZCLAW_MONITOR_REPORTS/binary-build.txt"
"${compose[@]}" exec -T server curl -fsS http://127.0.0.1:9820/server-info > "$GIZCLAW_MONITOR_REPORTS/server-info.json"
"${compose[@]}" exec -T edge curl -fsS http://127.0.0.1:9821/server-info > "$GIZCLAW_MONITOR_REPORTS/edge-info.json"
sleep 3
date -u +%FT%TZ > "$GIZCLAW_MONITOR_REPORTS/before.time"
run_status=0
for round in 1 2 3; do
  "${compose[@]}" run --rm test /src/tests/gizclaw-e2e/testdata/observer-lifecycle/first-response.giztest.yaml --parallel 3 --output "/reports/first-response-$round.json" || run_status=1
  date -u +%FT%TZ > "$GIZCLAW_MONITOR_REPORTS/after-first-$round.time"
done
"${compose[@]}" run --rm test \
  /src/tests/gizclaw-e2e/testdata/observer-lifecycle/disconnect.giztest.yaml \
  /src/tests/gizclaw-e2e/testdata/observer-lifecycle/run-stop.giztest.yaml \
  /src/tests/gizclaw-e2e/testdata/observer-lifecycle/workspace-delete.giztest.yaml \
  /src/tests/gizclaw-e2e/testdata/observer-lifecycle/peer-delete.giztest.yaml \
  --parallel 2 --output /reports/lifecycle.json || run_status=1
"${compose[@]}" run --rm test /src/tests/gizclaw-e2e/giztest/slow-tts.eino.realtime.giztest.yaml /src/tests/gizclaw-e2e/giztest/slow-tts.eino.push-to-talk.giztest.yaml --output /reports/slow-tts.json || run_status=1
date -u +%FT%TZ > "$GIZCLAW_MONITOR_REPORTS/after-all.time"
sleep 20
date -u +%FT%TZ > "$GIZCLAW_MONITOR_REPORTS/delayed.time"
python3 "$script_dir/testdata/observer-lifecycle/summarize.py" "$repo_dir" "$run_dir" || run_status=1
exit "$run_status"
