#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/docker/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/edge-workspace"
pid_file="$workspace_dir/gizclaw-edge.pid"
log_file="$workspace_dir/gizclaw-edge.log"
ready_file="/tmp/gizclaw-e2e-edge-ready"
bin_path="$repo_root/tests/gizclaw-e2e/testdata/bin/gizclaw"
config_template="$repo_root/tests/gizclaw-e2e/testdata/edge-workspace/config.yaml.template"
# shellcheck disable=SC2016 # envsubst needs literal variable names.
envsubst_variables='${GIZCLAW_E2E_SERVER_ENDPOINT} ${GIZCLAW_E2E_EDGE_ENDPOINT} ${GIZCLAW_E2E_GATEWAY_ENDPOINT} ${GIZCLAW_E2E_EDGE_PRIVATE_KEY} ${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT} ${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY}'

cd "$repo_root"
rm -f "$ready_file"

: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
: "${GIZCLAW_E2E_EDGE_ENDPOINT:?missing GIZCLAW_E2E_EDGE_ENDPOINT}"
: "${GIZCLAW_E2E_GATEWAY_ENDPOINT:?missing GIZCLAW_E2E_GATEWAY_ENDPOINT}"
: "${GIZCLAW_E2E_EDGE_PRIVATE_KEY:=65kcbmbz2Eo2SgSA2Q3QqBoLhyLvYyMuZ7gfTSKaTvpR}"
: "${GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT:?missing GIZCLAW_E2E_EDGE_UPSTREAM_ENDPOINT}"
: "${GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY:=BoYfN5LcjihD8j7HmzDW56s3E9F2R1AX8JsucW5Zvd7T}"
export GIZCLAW_E2E_EDGE_PRIVATE_KEY GIZCLAW_E2E_EDGE_UPSTREAM_PUBLIC_KEY

if [[ "${GIZCLAW_E2E_GATEWAY_RELAY:-}" == "1" ]]; then
  : "${GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP:?missing GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP}"
  : "${GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP:?missing GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP}"
  : "${GIZCLAW_E2E_GATEWAY_RELAY_REALM:?missing GIZCLAW_E2E_GATEWAY_RELAY_REALM}"
  : "${GIZCLAW_E2E_GATEWAY_RELAY_USERNAME:?missing GIZCLAW_E2E_GATEWAY_RELAY_USERNAME}"
  : "${GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL:?missing GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL}"
  : "${GIZCLAW_E2E_GATEWAY_MAX_SESSIONS:?missing GIZCLAW_E2E_GATEWAY_MAX_SESSIONS}"
  : "${GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS:?missing GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS}"
  : "${GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM:?missing GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM}"
  : "${GIZCLAW_E2E_GATEWAY_STREAMS_PER_UPSTREAM:?missing GIZCLAW_E2E_GATEWAY_STREAMS_PER_UPSTREAM}"
  : "${GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES:?missing GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES}"
  config_template="$repo_root/tests/gizclaw-e2e/testdata/edge-workspace/config.gateway-relay.yaml.template"
  # shellcheck disable=SC2016 # envsubst needs literal variable names.
  envsubst_variables+=' ${GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP} ${GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP} ${GIZCLAW_E2E_GATEWAY_RELAY_REALM} ${GIZCLAW_E2E_GATEWAY_RELAY_USERNAME} ${GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL} ${GIZCLAW_E2E_GATEWAY_MAX_SESSIONS} ${GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS} ${GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM} ${GIZCLAW_E2E_GATEWAY_STREAMS_PER_UPSTREAM} ${GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES}'
fi

envsubst "$envsubst_variables" \
  < "$config_template" \
  > "$workspace_dir/config.yaml"

"$setup_dir/build.sh" >/dev/null

if [[ "${GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY:-}" == "1" ]]; then
  for _ in {1..300}; do
    if [[ -f /run/gizclaw-gateway-fault/ready ]]; then
      break
    fi
    sleep 0.1
  done
  if [[ ! -f /run/gizclaw-gateway-fault/ready ]]; then
    echo "gateway relay fault boundary did not become ready" >&2
    exit 1
  fi
fi

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

# shellcheck disable=SC2329 # Invoked by signal traps.
shutdown_edge() {
  rm -f "$ready_file"
  if kill -0 "$pid" 2>/dev/null; then
    kill -TERM "$pid" 2>/dev/null || true
    # Gateway.Close may use the configured 30-second drain before closing its
    # physical upstream pool. Keep this below the capacity runner's 45-second
    # Compose stop bound while leaving enough time for a graceful close.
    for _ in {1..400}; do
      if ! kill -0 "$pid" 2>/dev/null; then
        wait "$pid" 2>/dev/null || true
        exit 0
      fi
      sleep 0.1
    done
    kill -KILL "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  exit 0
}
trap shutdown_edge TERM INT

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker edge exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
