#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/.testbench/tikv-$(date +%Y%m%d-%H%M%S)-$$}"
test_run='^TestTiKV'
collections=1
auxiliary=1
case "${2:-}" in
  "") ;;
  --migration-only) test_run='^TestTiKVRedisMigration$'; collections=0 ;;
  --presence-only) test_run='^TestTiKV$/^presence-pages$'; collections=0; auxiliary=0 ;;
  *) echo 'usage: run_tikv_tests.sh [evidence-dir] [--migration-only|--presence-only]' >&2; exit 2 ;;
esac
mkdir -p "$evidence/bin"
evidence="$(cd "$evidence" && pwd)"
export TIKV_E2E_BIN_DIR="$evidence/bin"
project="gizclaw-tikv-$(date +%s)-$$"
printf '%s\n' "$project" >"$evidence/project.txt"
compose_file="$repo_root/tests/store-e2e/docker-compose.tikv.yaml"
compose() { docker compose -p "$project" -f "$compose_file" "$@"; }
cleanup() {
  compose ps -a --format json >"$evidence/final-topology.json" 2>&1 || true
  compose logs --no-color >"$evidence/cluster.log" 2>&1 || true
  compose down --volumes --remove-orphans >"$evidence/cleanup.log" 2>&1
}
trap cleanup EXIT
architecture="$(docker info --format '{{.Architecture}}')"
case "$architecture" in
  arm64|aarch64) test_arch=arm64 ;;
  amd64|x86_64) test_arch=amd64 ;;
  *) printf 'Unsupported Docker architecture: %s\n' "$architecture" >&2; exit 1 ;;
esac
printf 'Building Linux/%s test binaries\n' "$test_arch"
GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 go build -o "$TIKV_E2E_BIN_DIR/replication-proxy" ./tests/store-e2e/tikv-replication-proxy
GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 go test -c -tags=store_e2e -o "$TIKV_E2E_BIN_DIR/store.test" ./tests/store-e2e
if [ "$auxiliary" -eq 1 ]; then
  GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 go test -c -o "$TIKV_E2E_BIN_DIR/kv.test" ./pkgs/store/kv
  GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 go build -o "$TIKV_E2E_BIN_DIR/gizclaw" ./cmd/gizclaw
fi
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
git status --short >"$evidence/source-status.txt"
go version >"$evidence/go-version.txt"
cp "$compose_file" tests/store-e2e/tikv-test.toml tests/store-e2e/tikv_test.go tests/store-e2e/tikv_presence_test.go tests/store-e2e/tikv_response_proxy_test.go "$evidence/"
compose up -d pd leader gate learner redis redis-cluster >"$evidence/startup.log" 2>&1
compose ps --format json >"$evidence/topology.json"
printf 'Running real TiKV KV and regional Learner replication-lag tests\n'
set +e
compose run --rm --no-deps tests -test.v -test.run "$test_run" -test.timeout=5m >"$evidence/test.log" 2>&1
status=$?
if [ "$status" -eq 0 ] && [ "$collections" -eq 1 ]; then
  compose run --rm --no-deps --entrypoint /test/kv.test tests \
    -test.v -test.run '^TestTiKVCollectionContracts$' -test.timeout=2m >"$evidence/collections.log" 2>&1
  status=$?
fi
if [ "$status" -eq 0 ] && [ "$auxiliary" -eq 1 ]; then
  compose run --rm --no-deps --entrypoint /test/kv.test tests \
    -test.v -test.run '^TestRedisMigrationDeadlineRealCluster$' -test.timeout=2m >"$evidence/migration-deadline.log" 2>&1
  status=$?
fi
set -e
cleanup
trap - EXIT
printf 'exit_code=%s\nevidence=%s\n' "$status" "$evidence"
cat "$evidence/test.log"
if [ -f "$evidence/collections.log" ]; then cat "$evidence/collections.log"; fi
if [ -f "$evidence/migration-deadline.log" ]; then cat "$evidence/migration-deadline.log"; fi
if [ "$status" -ne 0 ]; then tail -80 "$evidence/cluster.log"; fi
exit "$status"
