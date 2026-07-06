#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/server-workspace"
pid_file="$workspace_dir/gizclaw-server.pid"
log_file="$workspace_dir/gizclaw-server.log"
ready_file="/tmp/gizclaw-e2e-server-ready"

cd "$repo_root"
rm -f "$ready_file"

export GIZCLAW_E2E_CONFIG_HOME="${GIZCLAW_E2E_CONFIG_HOME:-$repo_root/tests/gizclaw-e2e/testdata/cmd-config-home}"
export GIZCLAW_E2E_SERVER_ADDR="${GIZCLAW_E2E_SERVER_ADDR:-0.0.0.0:9820}"
if [[ -n "${GIZCLAW_E2E_WEBRTC_NAT1TO1_IPS:-}" && "$GIZCLAW_E2E_WEBRTC_NAT1TO1_IPS" != */* ]]; then
  container_ip="$(hostname -i | awk '{print $1}')"
  if [[ -n "$container_ip" ]]; then
    export GIZCLAW_E2E_WEBRTC_NAT1TO1_IPS="${GIZCLAW_E2E_WEBRTC_NAT1TO1_IPS}/${container_ip}"
  fi
fi

perl -0pi -e 's/^endpoint:\s*[^\n]+/endpoint: 0.0.0.0:9820/m' "$workspace_dir/config.yaml"

"$setup_dir/build.sh" >/dev/null
runtime_ice_tcp_addr="${GIZCLAW_E2E_WEBRTC_ICE_TCP_ADDR:-}"
runtime_rpc_stream_reuse="${GIZCLAW_E2E_RPC_STREAM_REUSE:-1}"
unset GIZCLAW_E2E_WEBRTC_ICE_TCP_ADDR
unset GIZCLAW_WEBRTC_ICE_TCP_ADDR
unset GIZCLAW_RPC_STREAM_REUSE
"$setup_dir/reset_data.sh" reset

if [[ ! -f "$pid_file" ]]; then
  echo "gizclaw server pid file was not created: $pid_file" >&2
  exit 1
fi

pid="$(cat "$pid_file")"
if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
  echo "gizclaw server is not running after reset_data; log=$log_file" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
fi

if [[ -n "$runtime_ice_tcp_addr" || "$runtime_rpc_stream_reuse" == "1" ]]; then
  kill "$pid" 2>/dev/null || true
  for _ in {1..50}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done
  rm -f "$pid_file"
  if [[ -n "$runtime_ice_tcp_addr" ]]; then
    export GIZCLAW_E2E_WEBRTC_ICE_TCP_ADDR="$runtime_ice_tcp_addr"
  fi
  if [[ "$runtime_rpc_stream_reuse" == "1" ]]; then
    export GIZCLAW_RPC_STREAM_REUSE=1
  fi
  nohup "$repo_root/tests/gizclaw-e2e/testdata/bin/gizclaw" serve --force "$workspace_dir" >"$log_file" 2>&1 </dev/null &
  pid="$!"
  echo "$pid" >"$pid_file"
  for _ in {1..300}; do
    if curl -fsS --max-time 1 "http://127.0.0.1:9820/server-info" >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
fi

if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
  echo "gizclaw server is not running after runtime restart; log=$log_file" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
fi

echo "gizclaw e2e docker server ready pid=$pid log=$log_file"
touch "$ready_file"

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker server exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
