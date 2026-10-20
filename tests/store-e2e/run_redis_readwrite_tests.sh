#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/.testbench/redis-readwrite-$(date +%Y%m%d-%H%M%S)-$$}"
mkdir -p "$evidence"
evidence="$(cd "$evidence" && pwd)"
project="gizclaw-redis-readwrite-$(date +%s)-$$"
GIZCLAW_TEST_LIVEKIT_RTC_PORT="$(python3 - <<'PY'
import socket
with socket.socket() as tcp, socket.socket(type=socket.SOCK_DGRAM) as udp:
    tcp.bind(('127.0.0.1', 0))
    port = tcp.getsockname()[1]
    udp.bind(('127.0.0.1', port))
    print(port)
PY
)"
GIZCLAW_TEST_LIVEKIT_API_KEY=presence-e2e
GIZCLAW_TEST_LIVEKIT_API_SECRET="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
GIZCLAW_TEST_LIVEKIT_CONFIG="$(cat <<EOF
port: 7880
rtc:
  tcp_port: $GIZCLAW_TEST_LIVEKIT_RTC_PORT
  udp_port: $GIZCLAW_TEST_LIVEKIT_RTC_PORT
  use_external_ip: false
keys:
  $GIZCLAW_TEST_LIVEKIT_API_KEY: $GIZCLAW_TEST_LIVEKIT_API_SECRET
EOF
)"
export GIZCLAW_TEST_LIVEKIT_RTC_PORT GIZCLAW_TEST_LIVEKIT_API_KEY
export GIZCLAW_TEST_LIVEKIT_API_SECRET GIZCLAW_TEST_LIVEKIT_CONFIG
compose_file="$repo_root/tests/store-e2e/docker-compose.redis-readwrite.yaml"
compose() {
  docker compose -p "$project" -f "$compose_file" "$@"
}
cleanup() {
  compose logs --no-color >"$evidence/redis.log" 2>&1 || true
  compose down --volumes --remove-orphans >"$evidence/cleanup.log" 2>&1
}
trap cleanup EXIT
compose up -d --wait --wait-timeout 60 >"$evidence/startup.log" 2>&1
compose ps --format json >"$evidence/topology.json"
GIZCLAW_TEST_REDIS_DSN="redis://$(compose port primary 6379)/15"
GIZCLAW_TEST_REDIS_READ_DSN="redis://$(compose port replica 6379)/15"
export GIZCLAW_TEST_REDIS_DSN GIZCLAW_TEST_REDIS_READ_DSN
GIZCLAW_TEST_LIVEKIT_URL="ws://$(compose port livekit 7880)"
export GIZCLAW_TEST_LIVEKIT_URL
compose exec -T primary redis-cli info replication >"$evidence/primary-replication.txt"
compose exec -T replica redis-cli info replication >"$evidence/replica-replication.txt"
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
go version >"$evidence/go-version.txt"
cp "$compose_file" tests/store-e2e/redis_readwrite_test.go "$evidence/"
set +e
go test -tags=store_e2e -count=1 -p 1 -v -timeout 3m \
  -run '^(TestRedis|TestReadWriteRedis|TestIntegrationSharedRedisPresence)' \
  ./tests/store-e2e ./pkgs/gizclaw >"$evidence/test.log" 2>&1
status=$?
set -e
compose exec -T replica redis-cli info replication >"$evidence/replica-replication-final.txt"
cleanup
trap - EXIT
printf 'exit_code=%s\nevidence=%s\n' "$status" "$evidence"
cat "$evidence/test.log"
exit "$status"
