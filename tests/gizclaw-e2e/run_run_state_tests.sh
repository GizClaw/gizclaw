#!/usr/bin/env bash
# Local PostgreSQL/TLS + real Server/WebRTC/Giztest, deterministic AI providers.
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/tests/gizclaw-e2e/.testbench/run-state-$(date +%Y%m%d-%H%M%S)}"
mkdir -p "$evidence/overlay" "$evidence/tls"
evidence="$(cd "$evidence" && pwd)"
run_id="gizclaw-run-state-$(date +%s)-$$"
pg_image='gizclaw-history-postgres:fixture'
docker build -f build/postgres/Dockerfile -t "$pg_image" . >"$evidence/postgres-build.log" 2>&1
cleanup() {
  docker logs "$run_id-pg" >"$evidence/postgres.log" 2>&1 || true
  docker rm -f -v "$run_id-pg" >/dev/null 2>&1 || true
}
trap cleanup EXIT
openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj '/CN=localhost' \
  -keyout "$evidence/tls/server.key" -out "$evidence/tls/server.crt" \
  >"$evidence/tls/generation.log" 2>&1
docker run -d --name "$run_id-pg" --cpus 4 --memory 1g \
  --tmpfs '/var/lib/postgresql/data:rw,size=256m' \
  --mount "type=bind,source=$evidence/tls,target=/tls,readonly" \
  -e POSTGRES_USER=gizclaw -e POSTGRES_PASSWORD=local-run-state-fixture -e POSTGRES_DB=gizclaw \
  -p 127.0.0.1::5432 --entrypoint sh "$pg_image" -c \
  'cp /tls/server.key /tls/server.crt /tmp/ && chown postgres:postgres /tmp/server.key /tmp/server.crt && chmod 600 /tmp/server.key && exec /usr/local/bin/docker-entrypoint.sh "$@"' \
  -- postgres -c max_connections=300 -c ssl=on \
  -c ssl_cert_file=/tmp/server.crt -c ssl_key_file=/tmp/server.key \
  -c log_connections=on -c log_disconnections=on -c log_lock_waits=on -c deadlock_timeout=50ms \
  -c shared_preload_libraries=pg_stat_statements -c track_io_timing=on >/dev/null
for _ in {1..120}; do
  if docker exec "$run_id-pg" pg_isready -h 127.0.0.1 -U gizclaw -d gizclaw >/dev/null 2>&1; then break; fi
  sleep 0.5
done
docker exec "$run_id-pg" psql -U gizclaw -d gizclaw -v ON_ERROR_STOP=1 \
  -c 'CREATE EXTENSION pg_stat_statements' -c 'CREATE SCHEMA partman' -c 'CREATE EXTENSION pg_partman WITH SCHEMA partman' >"$evidence/postgres-setup.log"
GIZCLAW_TEST_POSTGRES_DSN="postgres://gizclaw:local-run-state-fixture@$(docker port "$run_id-pg" 5432)/gizclaw?sslmode=require"
export GIZCLAW_TEST_POSTGRES_DSN
export GIZCLAW_TEST_RUN_STATE=1
export GIZCLAW_TEST_RUN_STATE_CONCURRENCY="${GIZCLAW_TEST_RUN_STATE_CONCURRENCY:-64}"
export GOMAXPROCS="${GOMAXPROCS:-8}"
docker inspect "$run_id-pg" >"$evidence/container.json"
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
go version >"$evidence/go-version.txt"
docker exec "$run_id-pg" psql -U gizclaw -d gizclaw -Atc 'SELECT version()' >"$evidence/postgres-version.txt"
python3 tests/gizclaw-e2e/testdata/run-state-postgres/overlay.py "$repo_root" "$evidence/overlay"
# Compare only the owning factory while retaining the exact same harness and
# input path. The selected ref is recorded; this is not a full old-release run.
if [[ -n "${GIZCLAW_TEST_RUN_STATE_BASE_REF:-}" ]]; then
  git show "$GIZCLAW_TEST_RUN_STATE_BASE_REF:pkgs/store/storage/sqlx_postgresql.go" >"$evidence/overlay/postgresql.go"
  python3 - "$repo_root" "$evidence/overlay" <<'PY'
import json, pathlib, sys
root, output = map(pathlib.Path, sys.argv[1:])
path = output / 'overlay.json'
overlay = json.loads(path.read_text())
overlay['Replace'][str(root / 'pkgs/store/storage/sqlx_postgresql.go')] = str(output / 'postgresql.go')
path.write_text(json.dumps(overlay))
PY
  printf '%s\n' "$GIZCLAW_TEST_RUN_STATE_BASE_REF" >"$evidence/baseline-factory-ref.txt"
fi
go test -c -ldflags=-w -overlay "$evidence/overlay/overlay.json" -o "$evidence/server.test" ./cmd/internal/server
shasum -a 256 "$evidence/server.test" >"$evidence/binary.sha256"
mkdir -p "$evidence/harness"
cp cmd/internal/server/run_state_postgres_giztest_test.go cmd/internal/server/run_state_trace_test.go \
  tests/gizclaw-e2e/testdata/run-state-postgres/provider.go \
  tests/gizclaw-e2e/testdata/run-state-postgres/overlay.py \
  tests/gizclaw-e2e/setup/summarize_run_state.py tests/gizclaw-e2e/run_run_state_tests.sh "$evidence/harness/"
status=0
for mode in natural slow-connect; do
  export GIZCLAW_TEST_RUN_STATE_MODE="$mode"
  export GIZCLAW_TEST_RUN_STATE_EVIDENCE="$evidence/$mode"
  if ! (cd cmd/internal/server && "$evidence/server.test" \
    -test.run '^TestPostgreSQLRunStateGiztest$' -test.v -test.timeout 4m) >"$evidence/$mode.log" 2>&1; then
    status=1
  fi
done
python3 tests/gizclaw-e2e/setup/summarize_run_state.py "$evidence/natural" "$evidence/slow-connect" >"$evidence/comparison.json"
cleanup
trap - EXIT
exit "$status"
