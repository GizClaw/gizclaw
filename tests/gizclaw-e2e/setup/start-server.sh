#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
e2e_dir="$repo_root/tests/gizclaw-e2e"
testdata_dir="$repo_root/tests/gizclaw-e2e/testdata"
workspace_dir="$testdata_dir/server-workspace"
bin_path="$testdata_dir/bin/gizclaw"
pid_file="$workspace_dir/gizclaw-server.pid"
log_file="$workspace_dir/gizclaw-server.log"
env_file="$e2e_dir/.env"

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi

config_home="${GIZCLAW_E2E_CONFIG_HOME:-$testdata_dir/cmd-config-home}"
context_name="${GIZCLAW_E2E_ADMIN_CONTEXT:-admin}"

wait_ready() {
  for _ in {1..300}; do
    if ping_ready; then
      return 0
    fi
    sleep 0.1
  done
  echo "gizclaw e2e server did not become ready; log=$log_file" >&2
  return 1
}

ping_ready() {
  XDG_CONFIG_HOME="$config_home" "$bin_path" connect ping --context "$context_name" >/dev/null 2>&1 &
  local ping_pid="$!"
  for _ in {1..20}; do
    if ! kill -0 "$ping_pid" 2>/dev/null; then
      wait "$ping_pid"
      return $?
    fi
    sleep 0.1
  done
  kill "$ping_pid" 2>/dev/null || true
  wait "$ping_pid" 2>/dev/null || true
  return 124
}

if [[ ! -x "$bin_path" ]]; then
  "$script_dir/build.sh" >/dev/null
fi

if [[ -f "$pid_file" ]]; then
  pid="$(cat "$pid_file")"
  if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
    wait_ready
    echo "gizclaw e2e server already running: pid=$pid"
    exit 0
  fi
  rm -f "$pid_file"
fi

"$bin_path" migrate --workspace "$workspace_dir"
cd "$repo_root"
nohup "$bin_path" serve --force "$workspace_dir" >"$log_file" 2>&1 </dev/null &
pid="$!"
echo "$pid" >"$pid_file"
wait_ready
if ! kill -0 "$pid" 2>/dev/null; then
  echo "gizclaw e2e server exited after readiness; log=$log_file" >&2
  tail -40 "$log_file" >&2 || true
  rm -f "$pid_file"
  exit 1
fi
echo "gizclaw e2e server pid=$pid log=$log_file"
