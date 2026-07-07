#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
setup_dir="$repo_root/tests/gizclaw-e2e/docker/setup"
workspace_dir="$repo_root/tests/gizclaw-e2e/testdata/server-workspace"
pid_file="$workspace_dir/gizclaw-server.pid"
log_file="$workspace_dir/gizclaw-server.log"
ready_file="/tmp/gizclaw-e2e-server-ready"
bin_path="$repo_root/tests/gizclaw-e2e/testdata/bin/gizclaw"

cd "$repo_root"
rm -f "$ready_file"

export GIZCLAW_E2E_CONFIG_HOME="${GIZCLAW_E2E_CONFIG_HOME:-$repo_root/tests/gizclaw-e2e/testdata/cmd-config-home}"
: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
container_config_home="$GIZCLAW_E2E_CONFIG_HOME"
container_server_endpoint="$GIZCLAW_E2E_SERVER_ENDPOINT"
if [[ -f "$repo_root/tests/gizclaw-e2e/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$repo_root/tests/gizclaw-e2e/.env"
  set +a
fi
export GIZCLAW_E2E_CONFIG_HOME="$container_config_home"
export GIZCLAW_E2E_SERVER_ENDPOINT="$container_server_endpoint"
: "${GIZCLAW_E2E_VOLC_LOG_ENABLED:=false}"
: "${GIZCLAW_E2E_VOLC_LOG_ENDPOINT:=https://tls-cn-beijing.volces.com}"
: "${GIZCLAW_E2E_VOLC_LOG_REGION:=cn-beijing}"
: "${GIZCLAW_E2E_VOLC_LOG_TOPIC_ID:=gizclaw-server-log-topic}"
: "${GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_ID:=volc-access-key-id}"
: "${GIZCLAW_E2E_VOLC_LOG_ACCESS_KEY_SECRET:=volc-access-key-secret}"

envsubst '${GIZCLAW_E2E_SERVER_ENDPOINT}' \
  < "$repo_root/tests/gizclaw-e2e/docker/server-workspace.config.yaml.template" \
  > "$workspace_dir/config.yaml"
awk \
  -v enabled="$GIZCLAW_E2E_VOLC_LOG_ENABLED" \
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
/^  volc:/ { in_volc = 1; print; next }
in_volc && /^    enabled:/ { print "    enabled: " enabled; next }
in_volc && /^    endpoint:/ { print "    endpoint: " quote_yaml(endpoint); next }
in_volc && /^    region:/ { print "    region: " quote_yaml(region); next }
in_volc && /^    topic_id:/ { print "    topic_id: " quote_yaml(topic_id); next }
in_volc && /^    access_key_id:/ { print "    access_key_id: " quote_yaml(access_key_id); next }
in_volc && /^    access_key_secret:/ { print "    access_key_secret: " quote_yaml(access_key_secret); next }
in_volc && /^  [^ ]/ { in_volc = 0 }
{ print }
' "$workspace_dir/config.yaml" > "$workspace_dir/config.yaml.tmp"
mv "$workspace_dir/config.yaml.tmp" "$workspace_dir/config.yaml"

"$setup_dir/build.sh" >/dev/null
"$setup_dir/reset_data.sh" clear

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

"$setup_dir/reset_data.sh" init

echo "gizclaw e2e docker server ready pid=$pid log=$log_file"
touch "$ready_file"

while kill -0 "$pid" 2>/dev/null; do
  sleep 1
done

echo "gizclaw e2e docker server exited; log=$log_file" >&2
tail -120 "$log_file" >&2 || true
exit 1
