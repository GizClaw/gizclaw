#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
repo_root="$(cd "$e2e_dir/../.." && pwd)"
docker_dir="$e2e_dir/docker"
compose_file="$docker_dir/docker-compose.yaml"
volc_log_compose_file="$docker_dir/docker-compose.volc-log.yaml"
gateway_relay_compose_file="$docker_dir/docker-compose.gateway-relay.yaml"
env_file="$e2e_dir/.env"
state_root="$e2e_dir/testdata/docker"
# Keep a bounded relay pool large enough for one uninterrupted full run while
# still making port ownership and teardown assertions practical.
default_turn_relay_port_count=512

# shellcheck source=credentials.sh
# shellcheck disable=SC1091
source "$script_dir/credentials.sh"

stack_mode="standard"
topology_mode="full"
while (($# > 0)); do
  case "$1" in
    --volc-log)
      stack_mode="volc-log"
      shift
      ;;
    --gateway-capacity)
      topology_mode="gateway-capacity"
      shift
      ;;
    --gateway-capacity-direct)
      topology_mode="gateway-capacity-direct"
      shift
      ;;
    --gateway-native-channels-2048)
      topology_mode="gateway-native-channels-2048"
      shift
      ;;
    --gateway-relay-recovery)
      topology_mode="gateway-relay-recovery"
      shift
      ;;
    --firmware-only)
      export GIZCLAW_E2E_RESOURCE_PATHS="04-workflows/22-chatroom-direct.yaml 04-workflows/24-pet-chatroom.yaml 06-firmwares/00-devkit-main.yaml"
      export GIZCLAW_E2E_SYNC_VOLC_TENANT_ID=""
      shift
      ;;
    *)
      break
      ;;
  esac
done
if [[ "$topology_mode" != "gateway-relay-recovery" ]]; then
  require_gizclaw_e2e_credentials "$env_file"
fi
if [[ "$stack_mode" == "volc-log" ]]; then
  # shellcheck disable=SC2154
  require_gizclaw_e2e_credentials "$env_file" "${gizclaw_e2e_volc_log_credentials[@]}"
  : "${GIZCLAW_E2E_VOLC_LOG_ENDPOINT:?set the provisioned LogStore endpoint}"
  : "${GIZCLAW_E2E_VOLC_LOG_REGION:?set the provisioned LogStore region}"
  : "${GIZCLAW_E2E_VOLC_LOG_TOPIC_ID:?set the provisioned LogStore topic id}"
fi
if [[ "$topology_mode" == "gateway-capacity" || "$topology_mode" == "gateway-capacity-direct" || "$topology_mode" == "gateway-native-channels-2048" ]]; then
  export GIZCLAW_E2E_CAPACITY_ONLY=1
else
  unset GIZCLAW_E2E_CAPACITY_ONLY
fi

pick_gateway_relay_subnet() {
  local project="$1"
  local existing
  existing="$(docker network ls -q | xargs -n 50 docker network inspect --format '{{range .IPAM.Config}}{{println .Subnet}}{{end}}' 2>/dev/null || true)"
  python3 - "$project" "$existing" <<'PY'
import hashlib
import ipaddress
import sys

seed = int.from_bytes(hashlib.sha256(sys.argv[1].encode()).digest()[:4], "big")
used = []
for value in sys.argv[2].split():
    try:
        used.append(ipaddress.ip_network(value, strict=False))
    except ValueError:
        pass
for offset in range(12 * 256):
    index = (seed + offset) % (12 * 256)
    second = 20 + index // 256
    third = index % 256
    candidate = ipaddress.ip_network(f"172.{second}.{third}.0/24")
    if not any(candidate.overlaps(network) for network in used):
        print(candidate)
        break
else:
    raise SystemExit("no free project-scoped Docker /24 subnet")
PY
}

random_gateway_relay_value() {
  python3 - <<'PY'
import secrets
print(secrets.token_hex(24))
PY
}

write_gateway_relay_credentials() {
  local target="$1"
  : >"$target"
  local name
  # shellcheck disable=SC2154 # Declared by credentials.sh.
  for name in "${gizclaw_e2e_credentials[@]}"; do
    printf '%s=%s\n' "$name" "$(random_gateway_relay_value)" >>"$target"
  done
}

tcp_port_available() {
  local port="$1"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$port" <<'PY'
import socket
import sys

sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
try:
    try:
        sock.bind(("0.0.0.0", int(sys.argv[1])))
    except OSError:
        raise SystemExit(1)
finally:
    sock.close()
PY
    return
  fi
  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP@"*":"$port" >/dev/null 2>&1; then
      return 1
    fi
    return 0
  fi
  echo "checking TCP ports requires lsof or python3" >&2
  return 2
}

pick_free_tcp_port() {
  local port available_rc
  for _ in {1..100}; do
    port=$((20000 + RANDOM % 30000))
    if tcp_port_available "$port"; then
      echo "$port"
      return 0
    else
      available_rc=$?
      if [[ "$available_rc" == "2" ]]; then
        return 2
      fi
    fi
  done
  echo "failed to find a free local TCP port" >&2
  return 1
}

pick_free_udp_range() {
  local width="${1:-20}"
  shift || true
  local exclude_count="$#"
  local base port in_use
  for _ in {1..100}; do
    base=$((30000 + RANDOM % 20000))
    in_use=0
    for ((port = base; port < base + width; port++)); do
      if ((exclude_count > 0)); then
        local exclude
        for exclude in "$@"; do
          if [[ -n "$exclude" && "$port" == "$exclude" ]]; then
            in_use=1
            break
          fi
        done
      fi
      if [[ "$in_use" == "1" ]]; then
        break
      fi
      if udp_port_available "$port"; then
        continue
      else
        local available_rc=$?
        if [[ "$available_rc" == "2" ]]; then
          return 2
        fi
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

udp_port_available() {
  local port="$1"
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$port" <<'PY'
import socket
import sys

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
try:
    try:
        sock.bind(("0.0.0.0", int(sys.argv[1])))
    except OSError:
        raise SystemExit(1)
finally:
    sock.close()
PY
    return
  fi
	if command -v lsof >/dev/null 2>&1; then
		if lsof -nP -iUDP@"*":"$port" >/dev/null 2>&1; then
			return 1
		fi
		return 0
  fi
  echo "checking UDP ports requires lsof or python3" >&2
  return 2
}

pick_free_udp_port() {
  local exclude_min="$1"
  local exclude_max="$2"
  shift 2
  local port excluded exclude_port
  for _ in {1..100}; do
    port=$((20000 + RANDOM % 30000))
    if ((port >= exclude_min && port <= exclude_max)); then
      continue
    fi
    excluded=0
    for exclude_port in "$@"; do
      if [[ -n "$exclude_port" && "$port" == "$exclude_port" ]]; then
        excluded=1
        break
      fi
    done
    if [[ "$excluded" == "1" ]]; then
      continue
    fi
    if udp_port_available "$port"; then
      echo "$port"
      return 0
    fi
  done
  echo "failed to find a free local UDP port outside relay range $exclude_min-$exclude_max" >&2
  return 1
}

tcp_udp_port_available() {
  tcp_port_available "$1" && udp_port_available "$1"
}

pick_free_edge_port() {
  local exclude_min="$1"
  local exclude_max="$2"
  shift 2
  local port excluded exclude_port available_rc
  for _ in {1..100}; do
    port=$((20000 + RANDOM % 30000))
    if ((port >= exclude_min && port <= exclude_max)); then
      continue
    fi
    excluded=0
    for exclude_port in "$@"; do
      if [[ -n "$exclude_port" && "$port" == "$exclude_port" ]]; then
        excluded=1
        break
      fi
    done
    if [[ "$excluded" == "1" ]]; then
      continue
    fi
    if tcp_udp_port_available "$port"; then
      echo "$port"
      return 0
    else
      available_rc=$?
      if [[ "$available_rc" == "2" ]]; then
        return 2
      fi
    fi
  done
  echo "failed to find a free Edge TCP/UDP port outside relay range $exclude_min-$exclude_max" >&2
  return 1
}

detect_turn_host() {
  if [[ -n "${GIZCLAW_E2E_TURN_HOST:-}" ]]; then
    echo "$GIZCLAW_E2E_TURN_HOST"
    return 0
  fi
  local edge_host="${GIZCLAW_E2E_EDGE_HOST:-}"
  if [[ -n "$edge_host" && "$edge_host" != "127.0.0.1" && "$edge_host" != "localhost" && "$edge_host" != "::1" ]]; then
    echo "$edge_host"
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

docker_native_platform() {
  local platform
  if ! platform="$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}' 2>/dev/null)"; then
    echo "failed to determine Docker daemon platform" >&2
    return 1
  fi
  case "$platform" in
    linux/amd64 | linux/x86_64)
      echo "linux/amd64"
      ;;
    linux/arm64 | linux/aarch64)
      echo "linux/arm64"
      ;;
    *)
      echo "unsupported Docker daemon platform: ${platform:-unknown}; expected linux/amd64 or linux/arm64" >&2
      return 1
      ;;
  esac
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

rewrite_endpoint_config_file() {
  local file="$1"
  local endpoint="$2"
  if [[ ! -f "$file" ]]; then
    return 0
  fi
  GIZCLAW_REWRITE_ENDPOINT="$endpoint" \
    perl -0pi -e 's/^(\s*endpoint:\s*)[^\s]+/${1}$ENV{GIZCLAW_REWRITE_ENDPOINT}/mg' "$file"
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
GIZCLAW_E2E_EDGE_ENDPOINT=$GIZCLAW_E2E_EDGE_ENDPOINT
GIZCLAW_E2E_EDGE2_ENDPOINT=$GIZCLAW_E2E_EDGE2_ENDPOINT
GIZCLAW_E2E_TURN_ENDPOINT=$GIZCLAW_E2E_TURN_ENDPOINT
GIZCLAW_E2E_TURN_RELAY_ADDRESS=$GIZCLAW_E2E_TURN_RELAY_ADDRESS
GIZCLAW_E2E_TURN_REALM=$GIZCLAW_E2E_TURN_REALM
GIZCLAW_E2E_TURN_USERNAME=$GIZCLAW_E2E_TURN_USERNAME
GIZCLAW_E2E_TURN_CREDENTIAL=$GIZCLAW_E2E_TURN_CREDENTIAL
GIZCLAW_E2E_TURN_RELAY_MIN_PORT=$GIZCLAW_E2E_TURN_RELAY_MIN_PORT
GIZCLAW_E2E_TURN_RELAY_MAX_PORT=$GIZCLAW_E2E_TURN_RELAY_MAX_PORT
GIZCLAW_E2E_SERVER_PUBLIC_KEY=$server_public_key
GIZCLAW_E2E_DESKTOP_URL=$desktop_url
GIZCLAW_E2E_DOCKER_PROJECT=$GIZCLAW_E2E_DOCKER_PROJECT
GIZCLAW_E2E_DOCKER_ADMIN_PORT=$GIZCLAW_E2E_DOCKER_ADMIN_PORT
GIZCLAW_E2E_DOCKER_EDGE_PORT=$GIZCLAW_E2E_DOCKER_EDGE_PORT
GIZCLAW_E2E_DOCKER_EDGE2_PORT=$GIZCLAW_E2E_DOCKER_EDGE2_PORT
GIZCLAW_E2E_DOCKER_TURN_PORT=$GIZCLAW_E2E_DOCKER_TURN_PORT
GIZCLAW_E2E_DOCKER_COMPOSE_FILE=$compose_file
GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY=${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}
GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP=${GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP:-}
GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP=${GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP:-}
GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP=${GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP:-}
GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP=${GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP:-}
GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP=${GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP:-}
GIZCLAW_E2E_GATEWAY_RELAY_SUBNET=${GIZCLAW_E2E_GATEWAY_RELAY_SUBNET:-}
GIZCLAW_E2E_GATEWAY_RELAY_REALM=${GIZCLAW_E2E_GATEWAY_RELAY_REALM:-}
GIZCLAW_E2E_GATEWAY_RELAY_USERNAME=${GIZCLAW_E2E_GATEWAY_RELAY_USERNAME:-}
GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL=${GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL:-}
GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE=${GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE:-}
GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY=${GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY:-}
GIZCLAW_E2E_GATEWAY_RELAY_MODE=${GIZCLAW_E2E_GATEWAY_RELAY_MODE:-}
GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH=${GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH:-}
GIZCLAW_E2E_SINGLE_EDGE=${GIZCLAW_E2E_SINGLE_EDGE:-}
GIZCLAW_E2E_GATEWAY_MAX_SESSIONS=${GIZCLAW_E2E_GATEWAY_MAX_SESSIONS:-}
GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS=${GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS:-}
GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM=${GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM:-}
GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION=${GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION:-}
GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM=${GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM:-}
GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES=${GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES:-}
GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE=${GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE:-}
GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES=${GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES:-}
GIZCLAW_E2E_DOCKER_IMAGES_BUILT=${GIZCLAW_E2E_DOCKER_IMAGES_BUILT:-}
EOF
  cp "$state_dir/docker.env" "${GIZCLAW_E2E_DOCKER_ENV:-$state_root/current.env}"
}

materialize_runtime_config() {
  local state_dir="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT"
  local identities_home="$state_dir/identities"
  local config_home="$state_dir/cmd-config-home"

  rm -rf "$state_dir"
  mkdir -p "$state_dir"
  if [[ "$topology_mode" == "gateway-relay-recovery" ]]; then
    write_gateway_relay_credentials "$GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE"
  fi
  cp -R "$e2e_dir/testdata/identities" "$identities_home"
  cp -R "$e2e_dir/testdata/cmd-config-home" "$config_home"
  rewrite_endpoint_configs "$identities_home" "$GIZCLAW_E2E_EDGE_ENDPOINT"
  rewrite_endpoint_configs "$config_home" "$GIZCLAW_E2E_EDGE_ENDPOINT"
  rewrite_endpoint_config_file "$identities_home/${GIZCLAW_E2E_ADMIN_IDENTITY:-admin}/config.yaml" "$GIZCLAW_E2E_SERVER_ENDPOINT"
  rewrite_endpoint_config_file "$config_home/gizclaw/${GIZCLAW_E2E_ADMIN_CONTEXT:-admin}/config.yaml" "$GIZCLAW_E2E_SERVER_ENDPOINT"
  write_runtime_env "$state_dir" "$config_home" "$identities_home" ""
  echo "$state_dir/docker.env"
}

wait_http_ready() {
  local url="$1"
  local label="$2"
  local service="${3:-}"
  local attempt container_state
  for attempt in {1..300}; do
    if curl -fsS --max-time 1 "$url" >/dev/null 2>&1; then
      return 0
    fi
    container_state="unavailable"
    if [[ -n "$service" ]]; then
      local container_id exit_code
      container_id="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" ps --all -q "$service" 2>/dev/null || true)"
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
    if ((attempt % 75 == 0)); then
      echo "==> readiness heartbeat: check=http label=$label service=${service:-none} state=$container_state elapsed_seconds=$((attempt / 5)) url=$url"
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
  # Full deterministic provisioning includes the shared Workflow catalog and
  # both owner-uploaded icon formats, so allow the same five-minute startup
  # window as the server container health check.
  local attempt
  for attempt in {1..1500}; do
    local container_id container_state exit_code
    container_id="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" ps --all -q "$service" 2>/dev/null || true)"
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
    if ((attempt % 75 == 0)); then
      echo "==> readiness heartbeat: check=ready-file label=$label service=$service state=${container_state:-unavailable} elapsed_seconds=$((attempt / 5)) marker=$ready_file"
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

if [[ -z "${GIZCLAW_E2E_TURN_RELAY_MIN_PORT:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_MIN_PORT="$(pick_free_udp_range "$default_turn_relay_port_count")"
fi
if [[ -z "${GIZCLAW_E2E_TURN_RELAY_MAX_PORT:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_MAX_PORT=$((GIZCLAW_E2E_TURN_RELAY_MIN_PORT + default_turn_relay_port_count - 1))
fi
if [[ -z "${GIZCLAW_E2E_DOCKER_TURN_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_TURN_PORT="$(pick_free_udp_port "$GIZCLAW_E2E_TURN_RELAY_MIN_PORT" "$GIZCLAW_E2E_TURN_RELAY_MAX_PORT")"
fi
if [[ -z "${GIZCLAW_E2E_TURN_RELAY_ADDRESS:-}" ]]; then
  GIZCLAW_E2E_TURN_RELAY_ADDRESS="$(detect_turn_host)"
fi
if [[ -z "${GIZCLAW_E2E_DOCKER_EDGE_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_EDGE_PORT="$(pick_free_edge_port "$GIZCLAW_E2E_TURN_RELAY_MIN_PORT" "$GIZCLAW_E2E_TURN_RELAY_MAX_PORT" "$GIZCLAW_E2E_DOCKER_TURN_PORT")"
fi
if [[ -z "${GIZCLAW_E2E_DOCKER_EDGE2_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_EDGE2_PORT="$(pick_free_edge_port "$GIZCLAW_E2E_TURN_RELAY_MIN_PORT" "$GIZCLAW_E2E_TURN_RELAY_MAX_PORT" "$GIZCLAW_E2E_DOCKER_TURN_PORT" "$GIZCLAW_E2E_DOCKER_EDGE_PORT")"
fi
if [[ -z "${GIZCLAW_E2E_DOCKER_ADMIN_PORT:-}" ]]; then
  GIZCLAW_E2E_DOCKER_ADMIN_PORT="$(pick_free_tcp_port)"
fi
if ((GIZCLAW_E2E_DOCKER_TURN_PORT >= GIZCLAW_E2E_TURN_RELAY_MIN_PORT &&
  GIZCLAW_E2E_DOCKER_TURN_PORT <= GIZCLAW_E2E_TURN_RELAY_MAX_PORT)); then
  echo "TURN listener port overlaps relay range: $GIZCLAW_E2E_DOCKER_TURN_PORT in $GIZCLAW_E2E_TURN_RELAY_MIN_PORT-$GIZCLAW_E2E_TURN_RELAY_MAX_PORT" >&2
  exit 2
fi
if ! udp_port_available "$GIZCLAW_E2E_DOCKER_TURN_PORT"; then
  echo "TURN listener UDP port is unavailable: $GIZCLAW_E2E_DOCKER_TURN_PORT" >&2
  exit 2
fi
if [[ "$GIZCLAW_E2E_DOCKER_ADMIN_PORT" == "$GIZCLAW_E2E_DOCKER_EDGE_PORT" ||
  "$GIZCLAW_E2E_DOCKER_ADMIN_PORT" == "$GIZCLAW_E2E_DOCKER_EDGE2_PORT" ||
  "$GIZCLAW_E2E_DOCKER_EDGE_PORT" == "$GIZCLAW_E2E_DOCKER_EDGE2_PORT" ||
  "$GIZCLAW_E2E_DOCKER_EDGE_PORT" == "$GIZCLAW_E2E_DOCKER_TURN_PORT" ||
  "$GIZCLAW_E2E_DOCKER_EDGE2_PORT" == "$GIZCLAW_E2E_DOCKER_TURN_PORT" ]]; then
  echo "Server, Edge, and TURN listener ports must not collide" >&2
  exit 2
fi
if ((GIZCLAW_E2E_DOCKER_EDGE_PORT >= GIZCLAW_E2E_TURN_RELAY_MIN_PORT &&
  GIZCLAW_E2E_DOCKER_EDGE_PORT <= GIZCLAW_E2E_TURN_RELAY_MAX_PORT)) ||
  ((GIZCLAW_E2E_DOCKER_EDGE2_PORT >= GIZCLAW_E2E_TURN_RELAY_MIN_PORT &&
    GIZCLAW_E2E_DOCKER_EDGE2_PORT <= GIZCLAW_E2E_TURN_RELAY_MAX_PORT)); then
  echo "Edge listener port overlaps the TURN relay range" >&2
  exit 2
fi
if ! tcp_udp_port_available "$GIZCLAW_E2E_DOCKER_EDGE_PORT" ||
  ! tcp_udp_port_available "$GIZCLAW_E2E_DOCKER_EDGE2_PORT"; then
  echo "Edge listener port must be available for both TCP and UDP" >&2
  exit 2
fi
if [[ -z "${GIZCLAW_E2E_SERVER_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_SERVER_ENDPOINT="${GIZCLAW_E2E_SERVER_HOST:-127.0.0.1}:$GIZCLAW_E2E_DOCKER_ADMIN_PORT"
fi
if [[ -z "${GIZCLAW_E2E_EDGE_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_EDGE_ENDPOINT="${GIZCLAW_E2E_EDGE_HOST:-$GIZCLAW_E2E_TURN_RELAY_ADDRESS}:$GIZCLAW_E2E_DOCKER_EDGE_PORT"
fi
if [[ -z "${GIZCLAW_E2E_EDGE2_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_EDGE2_ENDPOINT="${GIZCLAW_E2E_EDGE_HOST:-$GIZCLAW_E2E_TURN_RELAY_ADDRESS}:$GIZCLAW_E2E_DOCKER_EDGE2_PORT"
fi
if [[ -z "${GIZCLAW_E2E_TURN_ENDPOINT:-}" ]]; then
  GIZCLAW_E2E_TURN_ENDPOINT="${GIZCLAW_E2E_TURN_RELAY_ADDRESS}:$GIZCLAW_E2E_DOCKER_TURN_PORT"
fi
GIZCLAW_E2E_TURN_REALM="${GIZCLAW_E2E_TURN_REALM:-gizclaw-e2e-edge}"
GIZCLAW_E2E_TURN_USERNAME="${GIZCLAW_E2E_TURN_USERNAME:-gizclaw-e2e}"
GIZCLAW_E2E_TURN_CREDENTIAL="${GIZCLAW_E2E_TURN_CREDENTIAL:-gizclaw-e2e-turn}"
GIZCLAW_E2E_GATEWAY_RELAY_SUBNET="${GIZCLAW_E2E_GATEWAY_RELAY_SUBNET:-}"
GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP="${GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP:-}"
GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP="${GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP:-}"
GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP="${GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP:-}"
GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP="${GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP:-}"
GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP="${GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP:-}"
GIZCLAW_E2E_GATEWAY_RELAY_REALM="${GIZCLAW_E2E_GATEWAY_RELAY_REALM:-}"
GIZCLAW_E2E_GATEWAY_RELAY_USERNAME="${GIZCLAW_E2E_GATEWAY_RELAY_USERNAME:-}"
GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL="${GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL:-}"
GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY=""
GIZCLAW_E2E_GATEWAY_RELAY_MODE=""
GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH=""
GIZCLAW_E2E_SINGLE_EDGE=""
if [[ "$topology_mode" == "gateway-capacity" || "$topology_mode" == "gateway-capacity-direct" || "$topology_mode" == "gateway-native-channels-2048" || "$topology_mode" == "gateway-relay-recovery" ]]; then
  GIZCLAW_E2E_GATEWAY_RELAY_SUBNET="${GIZCLAW_E2E_GATEWAY_RELAY_SUBNET:-$(pick_gateway_relay_subnet "$GIZCLAW_E2E_DOCKER_PROJECT")}"
  gateway_relay_prefix="${GIZCLAW_E2E_GATEWAY_RELAY_SUBNET%.0/24}"
  GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP="${GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP:-$gateway_relay_prefix.20}"
  GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP="${GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP:-$gateway_relay_prefix.21}"
  GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP="${GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP:-$gateway_relay_prefix.22}"
  GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP="${GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP:-$gateway_relay_prefix.10}"
  GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP="${GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP:-$gateway_relay_prefix.11}"
  GIZCLAW_E2E_GATEWAY_RELAY_REALM="${GIZCLAW_E2E_GATEWAY_RELAY_REALM:-gizclaw-gateway-relay.invalid}"
  GIZCLAW_E2E_GATEWAY_RELAY_USERNAME="${GIZCLAW_E2E_GATEWAY_RELAY_USERNAME:-relay-$(random_gateway_relay_value)}"
  GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL="${GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL:-$(random_gateway_relay_value)}"
  if [[ "$topology_mode" == "gateway-relay-recovery" ]]; then
    GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY="1"
    GIZCLAW_E2E_GATEWAY_RELAY_MODE="1"
    GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="relay"
    GIZCLAW_E2E_SINGLE_EDGE="1"
    GIZCLAW_E2E_GATEWAY_MAX_SESSIONS=8
    GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS=1
    GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM=8
    GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION=32
    GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM=32
    GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES=8
    GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT/gateway-relay.env"
  else
    if [[ "$topology_mode" == "gateway-capacity-direct" || "$topology_mode" == "gateway-native-channels-2048" ]]; then
      GIZCLAW_E2E_GATEWAY_RELAY_MODE="0"
      GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="direct"
    else
      GIZCLAW_E2E_GATEWAY_RELAY_MODE="1"
      GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH="relay"
    fi
    GIZCLAW_E2E_GATEWAY_MAX_SESSIONS=30000
    GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS=16
    GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM=2048
    GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION=32
    GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM=8192
    GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES=512
    GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE="$env_file"
    if [[ "$topology_mode" == "gateway-native-channels-2048" ]]; then
      GIZCLAW_E2E_SINGLE_EDGE="1"
      GIZCLAW_E2E_GATEWAY_MAX_SESSIONS=2048
      GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS=1
    fi
  fi
  GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY="$gateway_relay_compose_file"
else
  GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE=""
  GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY=""
fi
export GIZCLAW_E2E_DOCKER_PROJECT GIZCLAW_E2E_DOCKER_ADMIN_PORT GIZCLAW_E2E_DOCKER_EDGE_PORT GIZCLAW_E2E_DOCKER_EDGE2_PORT
export GIZCLAW_E2E_DOCKER_TURN_PORT
export GIZCLAW_E2E_SERVER_ENDPOINT GIZCLAW_E2E_EDGE_ENDPOINT GIZCLAW_E2E_EDGE2_ENDPOINT
export GIZCLAW_E2E_TURN_ENDPOINT GIZCLAW_E2E_TURN_RELAY_ADDRESS GIZCLAW_E2E_TURN_REALM GIZCLAW_E2E_TURN_USERNAME GIZCLAW_E2E_TURN_CREDENTIAL
export GIZCLAW_E2E_TURN_RELAY_MIN_PORT GIZCLAW_E2E_TURN_RELAY_MAX_PORT
export GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY
export GIZCLAW_E2E_GATEWAY_RELAY_SUBNET GIZCLAW_E2E_GATEWAY_RELAY_SERVER_IP GIZCLAW_E2E_GATEWAY_RELAY_EDGE_IP GIZCLAW_E2E_GATEWAY_RELAY_EDGE2_IP
export GIZCLAW_E2E_GATEWAY_RELAY_TURN_A_IP GIZCLAW_E2E_GATEWAY_RELAY_TURN_B_IP
export GIZCLAW_E2E_GATEWAY_RELAY_REALM GIZCLAW_E2E_GATEWAY_RELAY_USERNAME GIZCLAW_E2E_GATEWAY_RELAY_CREDENTIAL
export GIZCLAW_E2E_GATEWAY_RELAY_ENV_FILE
export GIZCLAW_E2E_GATEWAY_RELAY_RECOVERY GIZCLAW_E2E_SINGLE_EDGE
export GIZCLAW_E2E_GATEWAY_RELAY_MODE GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH
export GIZCLAW_E2E_GATEWAY_MAX_SESSIONS GIZCLAW_E2E_GATEWAY_MAX_UPSTREAMS
export GIZCLAW_E2E_GATEWAY_SESSIONS_PER_UPSTREAM GIZCLAW_E2E_GATEWAY_CHANNELS_PER_SESSION GIZCLAW_E2E_GATEWAY_CHANNELS_PER_UPSTREAM
export GIZCLAW_E2E_GATEWAY_MAX_PENDING_HANDSHAKES
export GIZCLAW_E2E_DOCKER_ADMIN_BIND="${GIZCLAW_E2E_DOCKER_ADMIN_BIND:-127.0.0.1}"
export GIZCLAW_E2E_DOCKER_SERVER_BIND="${GIZCLAW_E2E_DOCKER_SERVER_BIND:-0.0.0.0}"

capacity_build_required=0
if [[ "$topology_mode" == "gateway-capacity" || "$topology_mode" == "gateway-capacity-direct" || "$topology_mode" == "gateway-native-channels-2048" || "$topology_mode" == "gateway-relay-recovery" ]]; then
  GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE="${GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE:-$GIZCLAW_E2E_DOCKER_PROJECT-service}"
  if [[ "${GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES:-}" == "1" ]] &&
    docker image inspect "$GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE" >/dev/null 2>&1; then
    GIZCLAW_E2E_DOCKER_IMAGES_BUILT=0
    echo "==> reuse capacity service image: $GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE"
  else
    capacity_build_required=1
    GIZCLAW_E2E_DOCKER_IMAGES_BUILT=1
    echo "==> build capacity service image: $GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE"
  fi
  export GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE GIZCLAW_E2E_DOCKER_IMAGES_BUILT
fi

docker_platform="$(docker_native_platform)"
export DOCKER_DEFAULT_PLATFORM="$docker_platform"
platform_slug="${docker_platform//\//-}"
base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:${platform_slug}-cn-base}"
if ! docker image inspect "$base_image" >/dev/null 2>&1; then
  echo "==> build e2e Docker base $base_image for $docker_platform"
  docker build --platform="$docker_platform" -f "$repo_root/build/Dockerfile.cn.base" -t "$base_image" "$repo_root/build"
fi
export GIZCLAW_E2E_DOCKER_BASE_IMAGE="$base_image"

if [[ "${capacity_build_required:-0}" == "1" ]]; then
  if [[ "${GIZCLAW_E2E_GATEWAY_LINUX_PREBUILT:-}" == "1" ]]; then
    if [[ ! -x "$e2e_dir/testdata/bin/gizclaw-linux" ]]; then
      echo "prebuilt Linux CGO CLI is missing: $e2e_dir/testdata/bin/gizclaw-linux" >&2
      exit 2
    fi
    echo "==> use prebuilt Linux CGO CLI for capacity image"
  else
    GIZCLAW_E2E_DOCKER_BASE_IMAGE="$base_image" bash "$script_dir/build-linux-cgo.sh"
    export GIZCLAW_E2E_GATEWAY_LINUX_PREBUILT=1
  fi
fi

docker_env="$(materialize_runtime_config)"
echo "==> docker e2e env: $docker_env"
echo "==> start Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT server=$GIZCLAW_E2E_SERVER_ENDPOINT edges=$GIZCLAW_E2E_EDGE_ENDPOINT,$GIZCLAW_E2E_EDGE2_ENDPOINT turn=$GIZCLAW_E2E_TURN_ENDPOINT relay=${GIZCLAW_E2E_TURN_RELAY_MIN_PORT}-${GIZCLAW_E2E_TURN_RELAY_MAX_PORT}"
compose_files=(-f "$compose_file")
if [[ "$stack_mode" == "volc-log" ]]; then
  compose_files+=(-f "$volc_log_compose_file")
fi
if [[ "$topology_mode" == "gateway-capacity" || "$topology_mode" == "gateway-capacity-direct" || "$topology_mode" == "gateway-native-channels-2048" || "$topology_mode" == "gateway-relay-recovery" ]]; then
  compose_files+=(-f "$gateway_relay_compose_file")
fi
if [[ "$topology_mode" == "gateway-capacity" || "$topology_mode" == "gateway-capacity-direct" ]]; then
  if [[ $# -gt 0 ]]; then
    echo "gateway capacity modes do not accept docker compose arguments" >&2
    exit 2
  fi
  if [[ "$capacity_build_required" == "1" ]]; then
    docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" build server
  fi
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" up -d --no-build turn server edge edge2 coturn-a coturn-b
elif [[ "$topology_mode" == "gateway-native-channels-2048" ]]; then
  if [[ $# -gt 0 ]]; then
    echo "--gateway-native-channels-2048 does not accept docker compose arguments" >&2
    exit 2
  fi
  if [[ "$capacity_build_required" == "1" ]]; then
    docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" build server
  fi
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" up -d --no-build turn server edge coturn-a coturn-b
elif [[ "$topology_mode" == "gateway-relay-recovery" ]]; then
  if [[ $# -gt 0 ]]; then
    echo "--gateway-relay-recovery does not accept docker compose arguments" >&2
    exit 2
  fi
  if [[ "$capacity_build_required" == "1" ]]; then
    docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" build server
  fi
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" up -d --no-build turn server edge coturn-a coturn-b gateway-fault
elif [[ $# -gt 0 ]]; then
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" up "$@"
else
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_files[@]}" up -d --build
fi

edge_tcp_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol tcp edge 9821 | awk -F: '{print $NF}')"
edge2_tcp_port=""
if [[ "$GIZCLAW_E2E_SINGLE_EDGE" != "1" ]]; then
  edge2_tcp_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port --protocol tcp edge2 9821 | awk -F: '{print $NF}')"
fi
desktop_url=""
if [[ "$topology_mode" == "full" ]]; then
  desktop_port="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" port desktop 4191 | awk -F: '{print $NF}')"
  desktop_url="http://127.0.0.1:${desktop_port}"
fi

wait_docker_ready_file "server" "/tmp/gizclaw-e2e-server-ready" "docker server"
wait_http_ready "http://$GIZCLAW_E2E_SERVER_ENDPOINT/server-info" "docker server admin" "server"
wait_http_ready "http://127.0.0.1:${edge_tcp_port}/server-info" "docker edge" "edge"
wait_docker_ready_file "edge" "/tmp/gizclaw-e2e-edge-ready" "docker edge"
if [[ "$GIZCLAW_E2E_SINGLE_EDGE" != "1" ]]; then
  wait_http_ready "http://127.0.0.1:${edge2_tcp_port}/server-info" "docker edge2" "edge2"
  wait_docker_ready_file "edge2" "/tmp/gizclaw-e2e-edge-ready" "docker edge2"
fi
server_public_key="$(fetch_server_public_key "http://127.0.0.1:${edge_tcp_port}/server-info")"
if [[ -z "$server_public_key" ]]; then
  echo "docker edge /server-info did not return server public_key" >&2
  exit 2
fi
if [[ "$topology_mode" == "full" ]]; then
  wait_http_ready "$desktop_url" "docker desktop" "desktop"
fi

state_dir="$state_root/$GIZCLAW_E2E_DOCKER_PROJECT"
write_runtime_env "$state_dir" "$state_dir/cmd-config-home" "$state_dir/identities" "$desktop_url" "$server_public_key"
echo "==> docker e2e ready: $state_dir/docker.env"
