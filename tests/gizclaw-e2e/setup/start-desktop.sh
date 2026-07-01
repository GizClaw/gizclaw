#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
testdata_dir="$repo_root/tests/gizclaw-e2e/testdata"
desktop_dir="$testdata_dir/desktop"
pid_file="$desktop_dir/gizclaw-desktop.pid"
log_file="$desktop_dir/gizclaw-desktop.log"
port="${GIZCLAW_E2E_DESKTOP_PORT:-4191}"

mkdir -p "$desktop_dir"
if [[ -f "$pid_file" ]]; then
  old_pid="$(cat "$pid_file")"
  if [[ -n "$old_pid" ]] && kill -0 "$old_pid" 2>/dev/null; then
    echo "desktop dev surface already running pid=$old_pid url=http://127.0.0.1:$port"
    exit 0
  fi
  rm -f "$pid_file"
fi

(cd "$repo_root/apps/wails/frontend" && nohup npm run dev -- --port "$port" >"$log_file" 2>&1 & echo $! >"$pid_file")
echo "http://127.0.0.1:$port"
