#!/usr/bin/env bash
set -euo pipefail

readonly ready_file="/tmp/gizclaw-coturn-ready"
readonly pid_file="/tmp/gizclaw-coturn.pid"
readonly config_dir="/tmp/gizclaw-coturn"
readonly config_file="$config_dir/turnserver.conf"

require_value() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "coturn fixture: missing $name" >&2
    exit 2
  fi
}

require_uint16() {
  local name="$1"
  require_value "$name"
  if [[ ! "${!name}" =~ ^[0-9]+$ ]] || ((10#${!name} < 1 || 10#${!name} > 65535)); then
    echo "coturn fixture: $name must be an integer between 1 and 65535" >&2
    exit 2
  fi
}

read_metrics() {
  local response
  if ! exec 3<>"/dev/tcp/127.0.0.1/${GIZCLAW_COTURN_METRICS_PORT}" 2>/dev/null; then
    return 1
  fi
  printf 'GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n' >&3
  response="$(cat <&3)"
  exec 3<&-
  exec 3>&-
  printf '%s\n' "$response"
}

metric_value() {
  local name="$1"
  read_metrics | awk -v metric="$name" '$1 == metric { print $2; found = 1; exit } END { if (!found) exit 1 }'
}

start_turnserver() {
  turnserver -c "$config_file" &
  turn_pid="$!"
  printf '%s\n' "$turn_pid" >"$pid_file"
}

wait_for_metrics() {
  local allocations
  for _ in {1..100}; do
    if ! kill -0 "$turn_pid" 2>/dev/null; then
      echo "coturn fixture: turnserver exited before readiness" >&2
      return 1
    fi
    if read_metrics >/dev/null 2>&1; then
      allocations="$(metric_value turn_total_allocations 2>/dev/null || printf '0\n')"
      printf '%s\n' "$allocations"
      return 0
    fi
    sleep 0.1
  done
  echo "coturn fixture: metrics did not become ready" >&2
  return 1
}

cleanup() {
  local status="$?"
  rm -f "$ready_file"
  if [[ -n "${turn_pid:-}" ]] && kill -0 "$turn_pid" 2>/dev/null; then
    kill -TERM "$turn_pid" 2>/dev/null || true
    wait "$turn_pid" 2>/dev/null || true
  fi
  rm -f "$config_file" "$pid_file"
  rmdir "$config_dir" 2>/dev/null || true
  exit "$status"
}
trap cleanup EXIT INT TERM

require_value GIZCLAW_COTURN_AUTH_MODE
require_value GIZCLAW_COTURN_REALM
require_uint16 GIZCLAW_COTURN_LISTEN_PORT
require_uint16 GIZCLAW_COTURN_RELAY_MIN_PORT
require_uint16 GIZCLAW_COTURN_RELAY_MAX_PORT
require_uint16 GIZCLAW_COTURN_METRICS_PORT
if ((10#${GIZCLAW_COTURN_RELAY_MIN_PORT} > 10#${GIZCLAW_COTURN_RELAY_MAX_PORT})); then
  echo "coturn fixture: relay port range is reversed" >&2
  exit 2
fi

private_ip="${GIZCLAW_COTURN_PRIVATE_IP:-}"
if [[ -z "$private_ip" ]]; then
  private_ip="$(hostname -i | awk '{ for (i = 1; i <= NF; i++) if ($i ~ /^[0-9]+\./) { print $i; exit } }')"
fi
if [[ ! "$private_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "coturn fixture: private address must be IPv4" >&2
  exit 2
fi
public_ip="${GIZCLAW_COTURN_PUBLIC_IP:-$private_ip}"
if [[ ! "$public_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "coturn fixture: public address must be IPv4" >&2
  exit 2
fi

case "$GIZCLAW_COTURN_AUTH_MODE" in
  static)
    require_value GIZCLAW_COTURN_USERNAME
    require_value GIZCLAW_COTURN_CREDENTIAL
    auth_config="user=${GIZCLAW_COTURN_USERNAME}:${GIZCLAW_COTURN_CREDENTIAL}"
    probe_auth=(-u "$GIZCLAW_COTURN_USERNAME" -w "$GIZCLAW_COTURN_CREDENTIAL")
    ;;
  rest)
    require_value GIZCLAW_COTURN_CREDENTIAL
    auth_config=$'use-auth-secret\nstatic-auth-secret='"$GIZCLAW_COTURN_CREDENTIAL"
    probe_auth=(-W "$GIZCLAW_COTURN_CREDENTIAL")
    if [[ -n "${GIZCLAW_COTURN_USERNAME:-}" ]]; then
      probe_auth+=(-u "$GIZCLAW_COTURN_USERNAME")
    fi
    ;;
  *)
    echo "coturn fixture: GIZCLAW_COTURN_AUTH_MODE must be static or rest" >&2
    exit 2
    ;;
esac

umask 077
mkdir -p "$config_dir"
external_ip="$private_ip"
if [[ "$public_ip" != "$private_ip" ]]; then
  external_ip="$public_ip/$private_ip"
fi
{
  printf 'listening-port=%s\n' "$GIZCLAW_COTURN_LISTEN_PORT"
  printf 'listening-ip=%s\n' "$private_ip"
  printf 'relay-ip=%s\n' "$private_ip"
  printf 'external-ip=%s\n' "$external_ip"
  printf 'min-port=%s\n' "$GIZCLAW_COTURN_RELAY_MIN_PORT"
  printf 'max-port=%s\n' "$GIZCLAW_COTURN_RELAY_MAX_PORT"
  printf 'realm=%s\n' "$GIZCLAW_COTURN_REALM"
  printf '%s\n' "$auth_config"
  printf '%s\n' \
    fingerprint \
    lt-cred-mech \
    no-tcp \
    no-tls \
    no-dtls \
    no-tcp-relay \
    no-cli \
    no-multicast-peers \
    prometheus \
    "prometheus-address=127.0.0.1" \
    "prometheus-port=$GIZCLAW_COTURN_METRICS_PORT" \
    "prometheus-path=/metrics" \
    no-stdout-log \
    "log-file=/tmp/gizclaw-coturn.log" \
    simple-log
} >"$config_file"

version_output="$(turnserver --version 2>&1 | tail -n 1)"
read -r version _ <<<"$version_output"
if [[ "$version" != "4.7.0" ]]; then
  echo "coturn fixture: turnserver version $version_output, want 4.7.0" >&2
  exit 1
fi
echo "coturn fixture version=$version mode=$GIZCLAW_COTURN_AUTH_MODE"

start_turnserver
allocations="$(wait_for_metrics)"

probe_address="${GIZCLAW_COTURN_PROBE_ADDRESS:-$private_ip}"
if ! turnutils_uclient -y -c -p "$GIZCLAW_COTURN_LISTEN_PORT" "${probe_auth[@]}" "$probe_address" >/dev/null 2>&1; then
  echo "coturn fixture: authenticated UDP readiness probe failed" >&2
  exit 1
fi

# turnutils_uclient does not actively deallocate its short-lived sessions.
# Restart the unexposed pre-ready child so the serving process begins from the
# same zero-allocation baseline that the tests later require after cleanup.
kill -TERM "$turn_pid"
wait "$turn_pid" 2>/dev/null || true
turn_pid=""
start_turnserver
current="$(wait_for_metrics)"
if [[ "$current" != "$allocations" ]]; then
  echo "coturn fixture: readiness allocation did not return to baseline" >&2
  exit 1
fi
touch "$ready_file"

wait "$turn_pid"
