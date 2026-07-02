#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/server-workspace"
pid_file="$workspace_dir/gizclaw-server.pid"
log_file="$workspace_dir/gizclaw-server.log"

cd "$repo_root"

export GIZCLAW_E2E_CONFIG_HOME="${GIZCLAW_E2E_CONFIG_HOME:-$repo_root/tests/gizclaw-e2e/testdata/cmd-config-home}"
export GIZCLAW_E2E_SERVER_ADDR="${GIZCLAW_E2E_SERVER_ADDR:-127.0.0.1:9820}"

perl -0pi -e 's/^endpoint:\s*[^\n]+/endpoint: 0.0.0.0:9820/m' "$workspace_dir/config.yaml"

"$setup_dir/build.sh" >/dev/null
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

echo "gizclaw e2e docker server ready pid=$pid log=$log_file"

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker server exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
