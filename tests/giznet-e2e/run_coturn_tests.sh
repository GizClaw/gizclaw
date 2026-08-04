#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker-compose.coturn.yaml"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
project="giznet-coturn-$(printf '%s' "$run_id" | tr '[:upper:]' '[:lower:]')"
project="${project//[^a-z0-9_-]/-}"
artifact_dir="$script_dir/testdata/coturn/$run_id"
artifact_path="${GIZNET_COTURN_ARTIFACT:-$artifact_dir/transport.json}"

cleanup() {
  local status="$?"
  docker compose -p "$project" -f "$compose_file" down --remove-orphans --volumes >/dev/null 2>&1 || status=1
  if docker ps -aq --filter "label=com.docker.compose.project=$project" | grep -q .; then
    echo "giznet coturn e2e: project containers remain after cleanup" >&2
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT

random_hex() {
  openssl rand -hex 24
}

udp_port_available() {
  python3 - "$1" <<'PY'
import socket
import sys

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
try:
    sock.bind(("0.0.0.0", int(sys.argv[1])))
finally:
    sock.close()
PY
}

pick_udp_range() {
  local width="$1"
  local base port available
  for _ in {1..200}; do
    base=$((30000 + RANDOM % 25000))
    available=1
    for ((port = base; port < base + width; port++)); do
      if ! udp_port_available "$port"; then
        available=0
        break
      fi
    done
    if [[ "$available" == "1" ]]; then
      printf '%s\n' "$base"
      return 0
    fi
  done
  echo "giznet coturn e2e: no free UDP range" >&2
  return 1
}

pick_udp_port() {
  local port
  for _ in {1..200}; do
    port=$((20000 + RANDOM % 10000))
    if udp_port_available "$port"; then
      printf '%s\n' "$port"
      return 0
    fi
  done
  echo "giznet coturn e2e: no free UDP listener port" >&2
  return 1
}

detect_public_ip() {
  local address
  if command -v ipconfig >/dev/null 2>&1; then
    for interface in en0 en1; do
      address="$(ipconfig getifaddr "$interface" 2>/dev/null || true)"
      if [[ -n "$address" ]]; then
        printf '%s\n' "$address"
        return 0
      fi
    done
  fi
  if command -v ip >/dev/null 2>&1; then
    address="$(ip route get 1.1.1.1 2>/dev/null | awk '/src/ { for (i = 1; i <= NF; i++) if ($i == "src") { print $(i + 1); exit } }')"
    if [[ -n "$address" ]]; then
      printf '%s\n' "$address"
      return 0
    fi
  fi
  echo "giznet coturn e2e: cannot detect an IPv4 host address" >&2
  return 1
}

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "giznet coturn e2e: Docker is required" >&2
  exit 1
fi
if ! command -v openssl >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1; then
  echo "giznet coturn e2e: openssl and python3 are required" >&2
  exit 1
fi

readonly relay_width=32
GIZNET_COTURN_PUBLIC_IP="$(detect_public_ip)"
GIZNET_COTURN_STATIC_LISTEN_PORT="$(pick_udp_port)"
export GIZNET_COTURN_PUBLIC_IP GIZNET_COTURN_STATIC_LISTEN_PORT
while :; do
  GIZNET_COTURN_REST_LISTEN_PORT="$(pick_udp_port)"
  if [[ "$GIZNET_COTURN_REST_LISTEN_PORT" != "$GIZNET_COTURN_STATIC_LISTEN_PORT" ]]; then
    export GIZNET_COTURN_REST_LISTEN_PORT
    break
  fi
done
GIZNET_COTURN_STATIC_RELAY_MIN_PORT="$(pick_udp_range "$relay_width")"
export GIZNET_COTURN_STATIC_RELAY_MAX_PORT=$((GIZNET_COTURN_STATIC_RELAY_MIN_PORT + relay_width - 1))
export GIZNET_COTURN_STATIC_RELAY_MIN_PORT
while :; do
  GIZNET_COTURN_REST_RELAY_MIN_PORT="$(pick_udp_range "$relay_width")"
  GIZNET_COTURN_REST_RELAY_MAX_PORT=$((GIZNET_COTURN_REST_RELAY_MIN_PORT + relay_width - 1))
  if ((GIZNET_COTURN_REST_RELAY_MAX_PORT < GIZNET_COTURN_STATIC_RELAY_MIN_PORT ||
       GIZNET_COTURN_REST_RELAY_MIN_PORT > GIZNET_COTURN_STATIC_RELAY_MAX_PORT)); then
    export GIZNET_COTURN_REST_RELAY_MIN_PORT GIZNET_COTURN_REST_RELAY_MAX_PORT
    break
  fi
done
export GIZNET_COTURN_STATIC_REALM="giznet-static.invalid"
GIZNET_COTURN_STATIC_USERNAME="static-$(random_hex)"
GIZNET_COTURN_STATIC_CREDENTIAL="$(random_hex)"
export GIZNET_COTURN_REST_REALM="giznet-rest.invalid"
GIZNET_COTURN_REST_KEY_ID="rest-$(random_hex)"
GIZNET_COTURN_REST_SECRET="$(random_hex)"
export GIZNET_COTURN_STATIC_USERNAME GIZNET_COTURN_STATIC_CREDENTIAL
export GIZNET_COTURN_REST_KEY_ID GIZNET_COTURN_REST_SECRET

mkdir -p "$artifact_dir" "$(dirname "$artifact_path")"
artifact_dir="$(cd "$artifact_dir" && pwd -P)"
artifact_path="$(cd "$(dirname "$artifact_path")" && pwd -P)/$(basename "$artifact_path")"
export GIZNET_COTURN_DOCKER_PROJECT="$project"
export GIZNET_COTURN_COMPOSE_FILE="$compose_file"
export GIZNET_COTURN_STATIC_URL="turn:127.0.0.1:$GIZNET_COTURN_STATIC_LISTEN_PORT?transport=udp"
export GIZNET_COTURN_REST_URL="turn:127.0.0.1:$GIZNET_COTURN_REST_LISTEN_PORT?transport=udp"
export GIZNET_COTURN_ARTIFACT="$artifact_path"

echo "==> start pinned Coturn interoperability fixture"
docker compose -p "$project" -f "$compose_file" up -d --wait coturn-static coturn-rest

echo "==> run fixed Giznet Coturn interoperability and measurements"
(cd "$repo_root" && go test -count=1 -v \
  -tags 'giznet_e2e giznet_coturn_e2e' \
  ./tests/giznet-e2e/webrtc \
  -run '^(TestWebRTCCoturn|TestCoturnTransportMeasurements)$')

test -s "$GIZNET_COTURN_ARTIFACT"
echo "==> Giznet Coturn artifact: $GIZNET_COTURN_ARTIFACT"
