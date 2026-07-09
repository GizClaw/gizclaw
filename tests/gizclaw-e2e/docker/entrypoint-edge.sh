#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/docker/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/edge-workspace"
pid_file="$workspace_dir/gizclaw-edge.pid"
log_file="$workspace_dir/gizclaw-edge.log"
ready_file="/tmp/gizclaw-e2e-edge-ready"
bin_path="$repo_root/tests/gizclaw-e2e/testdata/bin/gizclaw"

cd "$repo_root"
rm -f "$ready_file"

: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
: "${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT:?missing GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT}"
: "${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY:=BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T}"
export GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY

envsubst '${GIZCLAW_E2E_SERVER_ENDPOINT} ${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT} ${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY}' \
  < "$repo_root/tests/gizclaw-e2e/testdata/edge-workspace/config.yaml.template" \
  > "$workspace_dir/config.yaml"

"$setup_dir/build.sh" >/dev/null

nohup "$bin_path" edge serve "$workspace_dir" >"$log_file" 2>&1 </dev/null &
pid="$!"
echo "$pid" >"$pid_file"

for _ in {1..300}; do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "gizclaw edge exited before becoming ready; log=$log_file" >&2
    tail -80 "$log_file" >&2 || true
    exit 1
  fi
  if curl -fsS --max-time 1 "http://127.0.0.1:9821/server-info" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
if ! curl -fsS --max-time 1 "http://127.0.0.1:9821/server-info" >/dev/null 2>&1; then
  echo "gizclaw edge did not become ready; log=$log_file" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
fi

echo "gizclaw e2e docker edge ready pid=$pid log=$log_file"
touch "$ready_file"

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker edge exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
