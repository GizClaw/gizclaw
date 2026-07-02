#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
testdata_dir="$repo_root/tests/gizclaw-e2e/testdata"
workspace_dir="$testdata_dir/server-workspace"
target="${1:-all}"
server_launch_label="com.gizclaw.e2e.server.$(printf '%s' "$repo_root" | cksum | awk '{print $1}')"

launchctl_supported() {
  [[ "$(uname -s)" == "Darwin" ]] && command -v launchctl >/dev/null 2>&1
}

stop_pid_file() {
  local name="$1"
  local pid_file="$2"
  if [[ ! -f "$pid_file" ]]; then
    return 0
  fi
  local pid
  pid="$(cat "$pid_file")"
  rm -f "$pid_file"
  if [[ -z "$pid" ]] || ! kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  kill "$pid" 2>/dev/null || true
  for _ in {1..50}; do
    if ! kill -0 "$pid" 2>/dev/null; then
      echo "stopped $name pid=$pid"
      return 0
    fi
    sleep 0.1
  done
  kill -9 "$pid" 2>/dev/null || true
  echo "killed $name pid=$pid"
}

case "$target" in
  all|server)
    if launchctl_supported; then
      launchctl remove "$server_launch_label" >/dev/null 2>&1 || true
    fi
    stop_pid_file "server" "$workspace_dir/gizclaw-server.pid"
    ;;
  *)
    echo "usage: $0 [all|server]" >&2
    exit 2
    ;;
esac
