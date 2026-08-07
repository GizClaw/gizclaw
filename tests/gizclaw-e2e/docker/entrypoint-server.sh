#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/docker/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/server-workspace"
pid_file="$workspace_dir/gizclaw-server.pid"
log_file="$workspace_dir/gizclaw-server.log"
ready_file="/tmp/gizclaw-e2e-server-ready"
http_ready_file="/tmp/gizclaw-e2e-server-http-ready"
bin_path="$repo_root/tests/gizclaw-e2e/testdata/bin/gizclaw"
server_mode="${1:-standard}"
case "$server_mode" in
  standard | volc-log) ;;
  *)
    echo "unsupported server mode: $server_mode" >&2
    exit 2
    ;;
esac

cd "$repo_root"
rm -f "$ready_file"
rm -f "$http_ready_file"

export GIZCLAW_E2E_CONFIG_HOME="${GIZCLAW_E2E_CONFIG_HOME:-$repo_root/tests/gizclaw-e2e/testdata/cmd-config-home}"
: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
: "${GIZCLAW_E2E_TURN_ENDPOINT:?missing GIZCLAW_E2E_TURN_ENDPOINT}"
: "${GIZCLAW_E2E_TURN_USERNAME:?missing GIZCLAW_E2E_TURN_USERNAME}"
: "${GIZCLAW_E2E_TURN_CREDENTIAL:?missing GIZCLAW_E2E_TURN_CREDENTIAL}"
container_config_home="$GIZCLAW_E2E_CONFIG_HOME"
container_server_endpoint="$GIZCLAW_E2E_SERVER_ENDPOINT"
container_turn_endpoint="$GIZCLAW_E2E_TURN_ENDPOINT"
container_turn_username="$GIZCLAW_E2E_TURN_USERNAME"
container_turn_credential="$GIZCLAW_E2E_TURN_CREDENTIAL"
# shellcheck source=../setup/credentials.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/credentials.sh"
require_gizclaw_e2e_credentials "$repo_root/tests/gizclaw-e2e/.env"
export GIZCLAW_E2E_CONFIG_HOME="$container_config_home"
export GIZCLAW_E2E_SERVER_ENDPOINT="$container_server_endpoint"
export GIZCLAW_E2E_TURN_ENDPOINT="$container_turn_endpoint"
export GIZCLAW_E2E_TURN_USERNAME="$container_turn_username"
export GIZCLAW_E2E_TURN_CREDENTIAL="$container_turn_credential"
# shellcheck disable=SC2016 # envsubst needs literal variable names.
envsubst '${GIZCLAW_E2E_SERVER_ENDPOINT} ${GIZCLAW_E2E_TURN_ENDPOINT} ${GIZCLAW_E2E_TURN_USERNAME} ${GIZCLAW_E2E_TURN_CREDENTIAL}' \
  < "$repo_root/tests/gizclaw-e2e/testdata/server-workspace/config.yaml.template" \
  > "$workspace_dir/config.yaml"
if [[ "${GIZCLAW_E2E_PERSISTENT_KV:-}" == "1" ]]; then
  perl -0pi -e 's/  memory:\n    kind: keyvalue\n    memory: \{\}/  persistent:\n    kind: keyvalue\n    badger:\n      dir: data\/state.badger/; s/storage: memory/storage: persistent/g' \
    "$workspace_dir/config.yaml"
fi
if [[ "${GIZCLAW_E2E_CAPACITY_ONLY:-}" == "1" ]]; then
  awk '
    /^ice-servers:/ { skip = 1; next }
    skip && /^edge-nodes:/ { skip = 0 }
    !skip { print }
  ' "$workspace_dir/config.yaml" > "$workspace_dir/config.yaml.tmp"
  mv "$workspace_dir/config.yaml.tmp" "$workspace_dir/config.yaml"
fi
if [[ "$server_mode" == "volc-log" ]]; then
  # shellcheck disable=SC2154
  require_gizclaw_e2e_credentials \
    "$repo_root/tests/gizclaw-e2e/.env" \
    "${gizclaw_e2e_volc_log_credentials[@]}"
  : "${GIZCLAW_E2E_VOLC_LOG_ENDPOINT:?missing GIZCLAW_E2E_VOLC_LOG_ENDPOINT}"
  : "${GIZCLAW_E2E_VOLC_LOG_REGION:?missing GIZCLAW_E2E_VOLC_LOG_REGION}"
  : "${GIZCLAW_E2E_VOLC_LOG_TOPIC_ID:?missing GIZCLAW_E2E_VOLC_LOG_TOPIC_ID}"
  : "${GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_ID:?missing GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_ID}"
  : "${GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_SECRET:?missing GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_SECRET}"
  awk \
    -v endpoint="$GIZCLAW_E2E_VOLC_LOG_ENDPOINT" \
    -v region="$GIZCLAW_E2E_VOLC_LOG_REGION" \
    -v topic_id="$GIZCLAW_E2E_VOLC_LOG_TOPIC_ID" \
    -v access_key_id="$GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_ID" \
    -v access_key_secret="$GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_SECRET" '
function quote_yaml(value) {
  gsub(/\\/, "\\\\", value)
  gsub(/"/, "\\\"", value)
  return "\"" value "\""
}
 /^system_log:/ {
  print "system_log:"
  print "  level: info"
  print "  query_store: logs"
  print "  sinks:"
  print "    - kind: stderr"
  print "    - kind: store"
  print "      store: logs"
  skip_system_log = 1
  next
}
skip_system_log && /^storage:/ { skip_system_log = 0 }
skip_system_log { next }
/^  peers:/ {
  print "  logs:"
  print "    kind: log"
  print "    volc:"
  print "      endpoint: " quote_yaml(endpoint)
  print "      region: " quote_yaml(region)
  print "      topic_id: " quote_yaml(topic_id)
  print "      access_key_id: " quote_yaml(access_key_id)
  print "      access_key_secret: " quote_yaml(access_key_secret)
}
{ print }
  ' "$workspace_dir/config.yaml" > "$workspace_dir/config.yaml.tmp"
  mv "$workspace_dir/config.yaml.tmp" "$workspace_dir/config.yaml"
fi

if [[ "${GIZCLAW_E2E_PREBUILT_CLI:-}" == "1" ]]; then
  if [[ ! -x "$bin_path" ]]; then
    echo "prebuilt Linux GizClaw CLI is missing: $bin_path" >&2
    exit 1
  fi
  echo "using prebuilt Linux GizClaw CLI: $bin_path"
else
  "$setup_dir/build.sh" >/dev/null
fi
# shellcheck disable=SC2016 # Perl replacement syntax is intentionally literal.
find "$GIZCLAW_E2E_CONFIG_HOME" -type f -name config.yaml -print0 |
  xargs -0 perl -0pi -e 's/^(\s*endpoint:\s*)[^\s]+/${1}127.0.0.1:9820/mg'
preserve_restart="${GIZCLAW_E2E_PRESERVE_DATA_ON_RESTART:-}"
initialized_file="$workspace_dir/.gizclaw-e2e-initialized"
if [[ "$preserve_restart" != "1" || ! -f "$initialized_file" ]]; then
  "$setup_dir/reset_data.sh" clear
fi

nohup "$bin_path" serve --force "$workspace_dir" >"$log_file" 2>&1 </dev/null &
pid="$!"
echo "$pid" >"$pid_file"

for _ in {1..300}; do
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "gizclaw server exited before becoming ready; log=$log_file" >&2
    tail -80 "$log_file" >&2 || true
    exit 1
  fi
  if curl -fsS --max-time 1 "http://127.0.0.1:9820/server-info" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
if ! curl -fsS --max-time 1 "http://127.0.0.1:9820/server-info" >/dev/null 2>&1; then
  echo "gizclaw server did not become ready; log=$log_file" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
fi

touch "$http_ready_file"
required_edge_urls=("http://edge:9821/server-info")
if [[ "${GIZCLAW_E2E_SINGLE_EDGE:-}" != "1" ]]; then
  required_edge_urls+=("http://edge2:9821/server-info")
fi
edges_ready() {
  local edge_url
  for edge_url in "${required_edge_urls[@]}"; do
    if ! curl -fsS --max-time 1 "$edge_url" >/dev/null 2>&1; then
      return 1
    fi
  done
  return 0
}
for _ in {1..600}; do
  if edges_ready; then
    break
  fi
  sleep 0.5
done
if ! edges_ready; then
  echo "gizclaw gateway edges did not become reachable from server before data init; log=$log_file" >&2
  tail -80 "$log_file" >&2 || true
  exit 1
fi

if [[ "${GIZCLAW_E2E_CAPACITY_ONLY:-}" != "1" && ( "$preserve_restart" != "1" || ! -f "$initialized_file" ) ]]; then
  "$setup_dir/reset_data.sh" init
fi
if [[ "$preserve_restart" == "1" ]]; then
  touch "$initialized_file"
fi

echo "gizclaw e2e docker server ready pid=$pid log=$log_file"
touch "$ready_file"

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker server exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
