#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
e2e_dir="$repo_root/tests/gizclaw-e2e"
testdata_dir="$repo_root/tests/gizclaw-e2e/testdata"
bin_path="$testdata_dir/bin/gizclaw"
env_file="$e2e_dir/.env"

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi

config_home="${GIZCLAW_E2E_CONFIG_HOME:-$testdata_dir/cmd-config-home}"
context_name="${GIZCLAW_E2E_CMD_GEAR1_CONTEXT:-gear1}"
pid_file="$testdata_dir/play-ui.pid"
log_file="$testdata_dir/play-ui.log"
listen_addr="127.0.0.1:8081"
ready_marker="GizClaw Play UI"

ui_ready() {
  curl -fsS "http://$listen_addr/" 2>/dev/null | grep -q "$ready_marker"
}

wait_ready() {
  for _ in {1..100}; do
    if [[ -n "${pid:-}" ]] && kill -0 "$pid" 2>/dev/null && ui_ready; then
      return 0
    fi
    sleep 0.1
  done
  echo "gizclaw e2e play UI did not become ready; log=$log_file" >&2
  tail -40 "$log_file" >&2 || true
  rm -f "$pid_file"
  exit 1
}

if [[ ! -x "$bin_path" ]]; then
  "$script_dir/build.sh" >/dev/null
fi

"$script_dir/stop.sh" play-ui >/dev/null || true

(
  cd "$repo_root"
  export XDG_CONFIG_HOME="$config_home"
  exec nohup "$bin_path" play --context "$context_name" --listen "$listen_addr"
) >"$log_file" 2>&1 </dev/null &
pid="$!"
echo "$pid" >"$pid_file"
wait_ready
echo "$pid" >"$pid_file"
echo "gizclaw e2e play UI pid=$pid url=http://$listen_addr log=$log_file"
