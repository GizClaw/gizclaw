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

  cat >"$state_dir/docker.env" <<EOF
GIZCLAW_E2E_CONFIG_HOME=$config_home
GIZCLAW_E2E_IDENTITIES_HOME=$identities_home
GIZCLAW_E2E_JS_IDENTITY_DIR=$identities_home/peer
GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR=$identities_home/admin
GIZCLAW_E2E_SERVER_ENDPOINT=$GIZCLAW_E2E_SERVER_ENDPOINT
GIZCLAW_E2E_DESKTOP_URL=$desktop_url
GIZCLAW_E2E_DOCKER_PROJECT=$GIZCLAW_E2E_DOCKER_PROJECT
GIZCLAW_E2E_DOCKER_SERVER_PORT=$GIZCLAW_E2E_DOCKER_SERVER_PORT
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

if [[ -z "${GIZCLAW_E2E_DOCKER_PROJECT:-}" ]]; then
  suffix="$(printf '%s-%s-%s' "${USER:-user}" "$(basename "$repo_root")" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
  GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-e2e-$suffix"
fi
validate_docker_project

if [[ -z "${GIZCLAW_E2E_DOCKER_SERVER_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_SERVER_PORT="$(pick_free_tcp_port)"
fi
if [[ -z "${GIZCLAW_E2E_SERVER_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_SERVER_ENDPOINT="${GIZCLAW_E2E_SERVER_HOST:-127.0.0.1}:$GIZCLAW_E2E_DOCKER_SERVER_PORT"
fi
export GIZCLAW_E2E_DOCKER_PROJECT GIZCLAW_E2E_DOCKER_SERVER_PORT GIZCLAW_E2E_SERVER_ENDPOINT
export GIZCLAW_E2E_DOCKER_SERVER_BIND="${GIZCLAW_E2E_DOCKER_SERVER_BIND:-0.0.0.0}"

base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-amd64-cn-base}"
if ! docker image inspect "$base_image" >/dev/null 2>&1; then
  echo "==> build e2e Docker base $base_image"
  docker build -f "$repo_root/build/Dockerfile.cn.base" -t "$base_image" "$repo_root/build"
fi
export GIZCLAW_E2E_DOCKER_BASE_IMAGE="$base_image"

docker_env="$(materialize_runtime_config)"
echo "==> docker e2e env: $docker_env"
echo "==> start Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT endpoint=$GIZCLAW_E2E_SERVER_ENDPOINT"
if [[ $# -gt 0 ]]; then
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" up "$@"
else
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" up -d --build
fi

server_tcp_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol tcp server 9820 | awk -F: '{print $NF}')"
server_udp_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol udp server 9820 | awk -F: '{print $NF}')"
if [[ "$server_tcp_port" != "$server_udp_port" ]]; then
  echo "docker server TCP/UDP port mismatch: tcp=$server_tcp_port udp=$server_udp_port" >&2
  exit 2
fi
desktop_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port desktop 4191 | awk -F: '{print $NF}')"
desktop_url="http://127.0.0.1:${desktop_port}"

wait_http_ready "http://127.0.0.1:${server_tcp_port}/server-info" "docker server" "server"
wait_docker_ready_file "server" "/tmp/gizclaw-e2e-server-ready" "docker server"
wait_http_ready "$desktop_url" "docker desktop" "desktop"

state_dir="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT"
write_runtime_env "$state_dir" "$state_dir/cmd-config-home" "$state_dir/identities" "$desktop_url"
echo "==> docker e2e ready: $state_dir/docker.env"
