#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/tests/gizclaw-e2e/.testbench/registration-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$evidence"
evidence="$(cd "$evidence" && pwd)"
run_id="gizclaw-registration-$(date +%s)-$$"
pg_image='postgres:17-alpine@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24'
redis_image='redis:8.4.6-alpine@sha256:12da49daa000c2be4d55118574f889c8cc298140ecf343d04f345c818f227823'
cleanup() {
  docker logs "$run_id-pg" >"$evidence/postgres.log" 2>&1 || true
  docker rm -f -v "$run_id-pg" "$run_id-redis" >/dev/null 2>&1 || true
}
trap 'cleanup' EXIT
pg_storage=()
if [[ "${GIZCLAW_TEST_REGISTRATION_PG_TMPFS:-0}" == "1" ]]; then
  pg_storage=(--tmpfs '/var/lib/postgresql/data:rw,size=256m')
fi
docker run -d --name "$run_id-pg" --cpus 2 --memory 1g \
  "${pg_storage[@]}" \
  -e POSTGRES_USER=gizclaw -e POSTGRES_PASSWORD=local-registration-fixture -e POSTGRES_DB=gizclaw \
  -p 127.0.0.1::5432 "$pg_image" \
  -c max_connections=300 -c log_lock_waits=on -c deadlock_timeout=50ms \
  -c shared_preload_libraries=pg_stat_statements -c track_io_timing=on >/dev/null
docker run -d --name "$run_id-redis" --cpus 1 --memory 256m \
  -p 127.0.0.1::6379 "$redis_image" redis-server --save '' --appendonly no >/dev/null
for _ in {1..120}; do
  if docker exec "$run_id-pg" pg_isready -h 127.0.0.1 -U gizclaw -d gizclaw >/dev/null 2>&1 && \
    docker exec "$run_id-redis" redis-cli ping >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
docker exec "$run_id-pg" psql -U gizclaw -d gizclaw -v ON_ERROR_STOP=1 \
  -c 'CREATE EXTENSION pg_stat_statements' >"$evidence/postgres-setup.log"
GIZCLAW_TEST_POSTGRES_DSN="postgres://gizclaw:local-registration-fixture@$(docker port "$run_id-pg" 5432)/gizclaw?sslmode=disable"
GIZCLAW_TEST_REDIS_URL="redis://$(docker port "$run_id-redis" 6379)/0"
export GIZCLAW_TEST_POSTGRES_DSN GIZCLAW_TEST_REDIS_URL
export GIZCLAW_TEST_REGISTRATION_EVIDENCE="$evidence"
export GIZCLAW_TEST_REGISTRATION_CONCURRENCY="${GIZCLAW_TEST_REGISTRATION_CONCURRENCY:-64}"
export GOMAXPROCS="${GOMAXPROCS:-8}"
docker inspect "$run_id-pg" "$run_id-redis" >"$evidence/containers.json"
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
go version >"$evidence/go-version.txt"
mkdir -p "$evidence/harness"
cp cmd/internal/server/registration_postgres_giztest_test.go \
  tests/gizclaw-e2e/run_registration_load_tests.sh \
  tests/gizclaw-e2e/setup/summarize_registration_load.py "$evidence/harness/"
docker exec "$run_id-pg" psql -U gizclaw -d gizclaw -Atc 'SELECT version()' >"$evidence/postgres-version.txt"
binary="${GIZCLAW_TEST_REGISTRATION_BINARY:-$evidence/server.test}"
if [[ -z "${GIZCLAW_TEST_REGISTRATION_BINARY:-}" ]]; then
  go test -c -ldflags=-w -o "$binary" ./cmd/internal/server
fi
shasum -a 256 "$binary" >"$evidence/binary.sha256"
printf 'concurrency=%s GOMAXPROCS=%s pg_tmpfs=%s\n' "$GIZCLAW_TEST_REGISTRATION_CONCURRENCY" "$GOMAXPROCS" "${GIZCLAW_TEST_REGISTRATION_PG_TMPFS:-0}" >"$evidence/settings.txt"
set +e
"$binary" -test.run '^TestPostgreSQLRegistrationGiztest$' -test.v -test.timeout 15m >"$evidence/test.log" 2>&1
status=$?
set -e
docker exec "$run_id-pg" psql -U gizclaw -d gizclaw -v ON_ERROR_STOP=1 --csv \
  -c "SELECT query,calls,total_exec_time,mean_exec_time,max_exec_time,rows,shared_blks_hit,shared_blks_read FROM pg_stat_statements WHERE query LIKE '%registration_%' ORDER BY total_exec_time DESC" >"$evidence/pg-stat-statements.csv"
python3 tests/gizclaw-e2e/setup/summarize_registration_load.py "$evidence"
cleanup
trap - EXIT
exit "$status"
