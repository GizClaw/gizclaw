#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: tests/integration/store/run_native.sh postgres|redis

Run integration tests against existing native services.

  postgres  Requires GIZCLAW_TEST_POSTGRES_DSN and PostgreSQL with pg_partman.
  redis     Requires GIZCLAW_TEST_REDIS_DSN and GIZCLAW_TEST_REDIS_READ_DSN
            for a Redis primary and replica in the same logical database.
  -h, --help, help  Show this help.
USAGE
}

if [[ $# -ne 1 ]]; then
  usage >&2
  exit 2
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$repo_root"

case "$1" in
  postgres)
    : "${GIZCLAW_TEST_POSTGRES_DSN:?GIZCLAW_TEST_POSTGRES_DSN is required}"
    GOFLAGS=-mod=readonly go test -count=1 -p 1 \
      ./cmd/internal/server \
      ./pkgs/store/memory \
      ./pkgs/gizclaw/services/ai/workspace \
      ./pkgs/gizclaw/services/runtime/graphstate \
      ./pkgs/gizclaw/services/runtime/peerrun \
      ./pkgs/gizclaw/services/system/runtimeprofile
    GOFLAGS=-mod=readonly go test -tags=store_e2e -count=1 -p 1 \
      -run '^TestPostgreSQL' \
      ./tests/integration/store
    GOFLAGS=-mod=readonly go test -count=1 -run '^TestPostgreSQL(HistoryRetentionWithoutWrites|InputPermission)' \
      ./pkgs/gizclaw
    ;;
  redis)
    : "${GIZCLAW_TEST_REDIS_DSN:?GIZCLAW_TEST_REDIS_DSN is required}"
    : "${GIZCLAW_TEST_REDIS_READ_DSN:?GIZCLAW_TEST_REDIS_READ_DSN is required}"
    GOFLAGS=-mod=readonly go test -race -count=1 -v -timeout 3m -run '^TestRedis' ./pkgs/store/kv
    GOFLAGS=-mod=readonly go test -tags=store_e2e -count=1 -p 1 -timeout 5m \
      -run '^(TestRedis|TestReadWriteRedis)' \
      ./tests/integration/store
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    printf 'Unknown backend: %s\n' "$1" >&2
    usage >&2
    exit 2
    ;;
esac
