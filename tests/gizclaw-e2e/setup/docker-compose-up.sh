#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
repo_root="$(cd "$e2e_dir/../.." && pwd)"
docker_dir="$e2e_dir/docker"
compose_file="$docker_dir/docker-compose.yaml"
env_file="$e2e_dir/.env"
state_root="$e2e_dir/testdata/docker"

if [[ ! -f "$env_file" ]]; then
  echo "missing $env_file; copy .env.example and fill provider credentials before Docker e2e" >&2
  exit 2
fi

pick_free_tcp_port() {
  local port
  for _ in {1..100}; do
    port=$((20000 + RANDOM % 30000))
    if ! (: >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1; then
      echo "$port"
      return 0
    fi
  done
  echo "failed to find a free local TCP port" >&2
  return 1
}

pick_free_udp_range() {
  local width="${1:-20}"
  local base port in_use
  for _ in {1..100}; do
    base=$((30000 + RANDOM % 20000))
    in_use=0
    for ((port = base; port < base + width; port++)); do
      if command -v lsof >/dev/null 2>&1 && lsof -nP -iUDP:"$port" >/dev/null 2>&1; then
        in_use=1
        break
      fi
    done
    if [[ "$in_use" == "0" ]]; then
      echo "$base"
      return 0
    fi
  done
  echo "failed to find a free local UDP relay port range" >&2
  return 1
}

detect_turn_host() {
  if [[ -n "${GIZCLAW_E2E_TURN_HOST:-}" ]]; then
    echo "$GIZCLAW_E2E_TURN_HOST"
    return 0
  fi
  local server_host="${GIZCLAW_E2E_SERVER_HOST:-}"
  if [[ -n "$server_host" && "$server_host" != "127.0.0.1" && "$server_host" != "localhost" && "$server_host" != "::1" ]]; then
    echo "$server_host"
    return 0
  fi
  if command -v ipconfig >/dev/null 2>&1; then
    for iface in en0 en1; do
      local addr
      addr="$(ipconfig getifaddr "$iface" 2>/dev/null || true)"
      if [[ -n "$addr" ]]; then
        echo "$addr"
        return 0
      fi
    done
  fi
  if command -v ip >/dev/null 2>&1; then
    local addr
    addr="$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ {for (i=1; i<=NF; i++) if ($i=="src") {print $(i+1); exit}}')"
    if [[ -n "$addr" ]]; then
      echo "$addr"
      return 0
    fi
  fi
  echo "failed to detect a TURN host address; set GIZCLAW_E2E_TURN_HOST" >&2
  return 1
}

validate_docker_project() {
  if [[ ! "$GIZCLAW_E2E_DOCKER_PROJECT" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
    echo "invalid GIZCLAW_E2E_DOCKER_PROJECT: $GIZCLAW_E2E_DOCKER_PROJECT" >&2
    echo "Docker Compose project names must start with a lowercase letter or digit and contain only lowercase letters, digits, underscores, or dashes." >&2
    exit 2
  fi
}

rewrite_endpoint_configs() {
  local root="$1"
  local endpoint="$2"
  local file
  while IFS= read -r file; do
    GIZCLAW_REWRITE_ENDPOINT="$endpoint" \
      perl -0pi -e 's/^(\s*endpoint:\s*)[^\s]+/${1}$ENV{GIZCLAW_REWRITE_ENDPOINT}/mg' "$file"
  done < <(find "$root" -type f -name config.yaml -print)
}

write_runtime_env() {
  local state_dir="$1"
  local config_home="$2"
  local identities_home="$3"
  local desktop_url="${4:-}"
  local server_public_key="${5:-}"

  cat >"$state_dir/docker.env" <<EOF
GIZCLAW_E2E_CONFIG_HOME=$config_home
GIZCLAW_E2E_IDENTITIES_HOME=$identities_home
GIZCLAW_E2E_JS_IDENTITY_DIR=$identities_home/peer
GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR=$identities_home/admin
GIZCLAW_E2E_SERVER_ENDPOINT=$GIZCLAW_E2E_SERVER_ENDPOINT
GIZCLAW_E2E_TURN_ENDPOINT=$GIZCLAW_E2E_TURN_ENDPOINT
GIZCLAW_E2E_TURN_RELAY_ADDRESS=$GIZCLAW_E2E_TURN_RELAY_ADDRESS
GIZCLAW_E2E_TURN_REALM=$GIZCLAW_E2E_TURN_REALM
GIZCLAW_E2E_TURN_USERNAME=$GIZCLAW_E2E_TURN_USERNAME
GIZCLAW_E2E_TURN_CREDENTIAL=$GIZCLAW_E2E_TURN_CREDENTIAL
GIZCLAW_E2E_TURN_RELAY_MIN_PORT=$GIZCLAW_E2E_TURN_RELAY_MIN_PORT
GIZCLAW_E2E_TURN_RELAY_MAX_PORT=$GIZCLAW_E2E_TURN_RELAY_MAX_PORT
GIZCLAW_E2E_SERVER_PUBLIC_KEY=$server_public_key
GIZCLAW_E2E_SKIP_PROVIDER_SYNC=${GIZCLAW_E2E_SKIP_PROVIDER_SYNC:-1}
GIZCLAW_E2E_DESKTOP_URL=$desktop_url
GIZCLAW_E2E_DOCKER_PROJECT=$GIZCLAW_E2E_DOCKER_PROJECT
GIZCLAW_E2E_DOCKER_SERVER_PORT=$GIZCLAW_E2E_DOCKER_SERVER_PORT
GIZCLAW_E2E_DOCKER_TURN_PORT=$GIZCLAW_E2E_DOCKER_TURN_PORT
GIZCLAW_E2E_DOCKER_COMPOSE_FILE=$compose_file
EOF
  cp "$state_dir/docker.env" "$state_root/current.env"
}

materialize_runtime_config() {
  local state_dir="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT"
  local identities_home="$state_dir/identities"
  local config_home="$state_dir/cmd-config-home"

  rm -rf "$state_dir"
  mkdir -p "$state_dir"
  cp -R "$e2e_dir/testdata/identities" "$identities_home"
  cp -R "$e2e_dir/testdata/cmd-config-home" "$config_home"
  rewrite_endpoint_configs "$identities_home" "$GIZCLAW_E2E_SERVER_ENDPOINT"
  rewrite_endpoint_configs "$config_home" "$GIZCLAW_E2E_SERVER_ENDPOINT"
  write_runtime_env "$state_dir" "$config_home" "$identities_home" ""
  echo "$state_dir/docker.env"
}

wait_http_ready() {
  local url="$1"
  local label="$2"
  local service="${3:-}"
  for _ in {1..300}; do
    if curl -fsS --max-time 1 "$url" >/dev/null 2>&1; then
      return 0
    fi
    if [[ -n "$service" ]]; then
      local container_id container_state exit_code
      container_id="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" ps -q "$service" 2>/dev/null || true)"
      if [[ -n "$container_id" ]]; then
        container_state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
        exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$container_id" 2>/dev/null || true)"
        if [[ "$container_state" == "exited" || "$container_state" == "dead" ]]; then
          echo "$label container exited before becoming ready at $url (state=$container_state exit=$exit_code)" >&2
          docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" logs --tail=200 "$service" >&2 || true
          return 1
        fi
      fi
    fi
    sleep 0.2
  done
  echo "$label did not become ready at $url" >&2
  if [[ -n "$service" ]]; then
    docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" logs --tail=200 "$service" >&2 || true
  fi
  return 1
}

wait_docker_ready_file() {
  local service="$1"
  local ready_file="$2"
  local label="$3"
  for _ in {1..300}; do
    local container_id container_state exit_code
    container_id="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" ps -q "$service" 2>/dev/null || true)"
    if [[ -n "$container_id" ]]; then
      container_state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
      exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$container_id" 2>/dev/null || true)"
      if [[ "$container_state" == "exited" || "$container_state" == "dead" ]]; then
        echo "$label container exited before ready marker $ready_file (state=$container_state exit=$exit_code)" >&2
        docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" logs --tail=200 "$service" >&2 || true
        return 1
      fi
      if docker exec "$container_id" test -f "$ready_file" >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep 0.2
  done
  echo "$label did not create ready marker $ready_file" >&2
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" logs --tail=200 "$service" >&2 || true
  return 1
}

fetch_server_public_key() {
  local url="$1"
  local body
  body="$(curl -fsS --max-time 2 "$url")"
  perl -0ne 'print "$1\n" if /"public_key"\s*:\s*"([^"]+)"/' <<<"$body"
}

if [[ -z "${GIZCLAW_E2E_DOCKER_PROJECT:-}" ]]; then
  suffix="$(printf '%s-%s-%s' "${USER:-user}" "$(basename "$repo_root")" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
  GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-e2e-$suffix"
fi
validate_docker_project

if [[ -z "${GIZCLAW_E2E_DOCKER_SERVER_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_SERVER_PORT="$(pick_free_tcp_port)"
fi
if [[ -z "${GIZCLAW_E2E_DOCKER_TURN_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_TURN_PORT="$(pick_free_tcp_port)"
fi
while [[ "$GIZCLAW_E2E_DOCKER_TURN_PORT" == "$GIZCLAW_E2E_DOCKER_SERVER_PORT" ]]; do
  GIZCLAW_E2E_DOCKER_TURN_PORT="$(pick_free_tcp_port)"
done
if [[ -z "${GIZCLAW_E2E_SERVER_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_SERVER_ENDPOINT="${GIZCLAW_E2E_SERVER_HOST:-127.0.0.1}:$GIZCLAW_E2E_DOCKER_SERVER_PORT"
fi
if [[ -z "${GIZCLAW_E2E_TURN_RELAY_MIN_PORT:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_MIN_PORT="$(pick_free_udp_range 20)"
fi
if [[ -z "${GIZCLAW_E2E_TURN_RELAY_MAX_PORT:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_MAX_PORT=$((GIZCLAW_E2E_TURN_RELAY_MIN_PORT + 19))
fi
if [[ -z "${GIZCLAW_E2E_TURN_RELAY_ADDRESS:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_ADDRESS="$(detect_turn_host)"
fi
if [[ -z "${GIZCLAW_E2E_TURN_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_TURN_ENDPOINT="${GIZCLAW_E2E_TURN_RELAY_ADDRESS}:$GIZCLAW_E2E_DOCKER_TURN_PORT"
fi
GIZCLAW_E2E_TURN_REALM="${GIZCLAW_E2E_TURN_REALM:-gizclaw-e2e-edge}"
GIZCLAW_E2E_TURN_USERNAME="${GIZCLAW_E2E_TURN_USERNAME:-gizclaw-e2e}"
GIZCLAW_E2E_TURN_CREDENTIAL="${GIZCLAW_E2E_TURN_CREDENTIAL:-gizclaw-e2e-turn}"
export GIZCLAW_E2E_DOCKER_PROJECT GIZCLAW_E2E_DOCKER_SERVER_PORT GIZCLAW_E2E_DOCKER_TURN_PORT GIZCLAW_E2E_SERVER_ENDPOINT
export GIZCLAW_E2E_TURN_ENDPOINT GIZCLAW_E2E_TURN_RELAY_ADDRESS GIZCLAW_E2E_TURN_REALM GIZCLAW_E2E_TURN_USERNAME GIZCLAW_E2E_TURN_CREDENTIAL
export GIZCLAW_E2E_TURN_RELAY_MIN_PORT GIZCLAW_E2E_TURN_RELAY_MAX_PORT
export GIZCLAW_E2E_DOCKER_SERVER_BIND="${GIZCLAW_E2E_DOCKER_SERVER_BIND:-0.0.0.0}"

base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-amd64-cn-base}"
if ! docker image inspect "$base_image" >/dev/null 2>&1; then
  echo "==> build e2e Docker base $base_image"
  docker build -f "$repo_root/build/Dockerfile.cn.base" -t "$base_image" "$repo_root/build"
fi
export GIZCLAW_E2E_DOCKER_BASE_IMAGE="$base_image"

docker_env="$(materialize_runtime_config)"
echo "==> docker e2e env: $docker_env"
echo "==> start Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT edge=$GIZCLAW_E2E_SERVER_ENDPOINT turn=$GIZCLAW_E2E_TURN_ENDPOINT relay=${GIZCLAW_E2E_TURN_RELAY_MIN_PORT}-${GIZCLAW_E2E_TURN_RELAY_MAX_PORT}"
if [[ $# -gt 0 ]]; then
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" up "$@"
else
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" up -d --build
fi

edge_tcp_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol tcp edge 9821 | awk -F: '{print $NF}')"
desktop_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port desktop 4191 | awk -F: '{print $NF}')"
desktop_url="http://127.0.0.1:${desktop_port}"

wait_docker_ready_file "server" "/tmp/gizclaw-e2e-server-ready" "docker server"
wait_http_ready "http://127.0.0.1:${edge_tcp_port}/server-info" "docker edge" "edge"
wait_docker_ready_file "edge" "/tmp/gizclaw-e2e-edge-ready" "docker edge"
server_public_key="$(fetch_server_public_key "http://127.0.0.1:${edge_tcp_port}/server-info")"
if [[ -z "$server_public_key" ]]; then
  echo "docker edge /server-info did not return server public_key" >&2
  exit 2
fi
if docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol tcp server 9820 >/tmp/gizclaw-e2e-server-port.out 2>/dev/null &&
  [[ -s /tmp/gizclaw-e2e-server-port.out ]]; then
  echo "docker server ICE port is unexpectedly published: $(cat /tmp/gizclaw-e2e-server-port.out)" >&2
  exit 2
fi
wait_http_ready "$desktop_url" "docker desktop" "desktop"

state_dir="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT"
write_runtime_env "$state_dir" "$state_dir/cmd-config-home" "$state_dir/identities" "$desktop_url" "$server_public_key"
echo "==> docker e2e ready: $state_dir/docker.env"
