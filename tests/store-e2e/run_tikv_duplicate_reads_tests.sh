#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
if [ "$#" -gt 1 ]; then
  echo 'usage: run_tikv_duplicate_reads_tests.sh [new-evidence-dir]' >&2
  exit 2
fi
evidence="${1:-$repo_root/.testbench/tikv-duplicate-reads-$(date +%Y%m%d-%H%M%S)-$$}"
if [ -e "$evidence" ]; then echo 'evidence directory must be new' >&2; exit 2; fi
mkdir -p "$evidence/bin"
evidence="$(cd "$evidence" && pwd)"
# Reuse the isolated three-voter benchmark fixture, without running its load.
export PRESENCE_BENCHMARK_DIR="$evidence"
compose_file="$repo_root/tests/store-e2e/docker-compose.presence-benchmark.yaml"
project="gizclaw-tikv-duplicate-reads-$(date +%s)-$$"
printf '%s\n' "$project" >"$evidence/project.txt"
compose() { docker compose -p "$project" -f "$compose_file" "$@"; }
collect() {
  local result=0
  compose ps -a --format json >"$evidence/final-topology.json" 2>&1 || result=1
  compose logs --no-color >"$evidence/cluster.log" 2>&1 || result=1
  return "$result"
}
cleanup() {
  collect || true
  compose down --timeout 3 --volumes --remove-orphans >"$evidence/cleanup.log" 2>&1
}
trap cleanup EXIT
architecture="$(docker info --format '{{.Architecture}}')"
case "$architecture" in
  arm64|aarch64) architecture=arm64 ;;
  amd64|x86_64) architecture=amd64 ;;
  *) echo 'unsupported Docker architecture' >&2; exit 2 ;;
esac
GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 go test -c -o "$evidence/bin/kv.test" ./pkgs/store/kv
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
git status --short >"$evidence/source-status.txt"
go version >"$evidence/go-version.txt"
go env GOFLAGS >"$evidence/go-flags.txt"
shasum -a 256 "$evidence/bin/kv.test" >"$evidence/binary-sha256.txt"
cp "$compose_file" tests/store-e2e/tikv-test.toml pkgs/store/kv/tikv_duplicate_reads_test.go "$evidence/"
compose up -d pd voter-a voter-b voter-c >"$evidence/startup.log" 2>&1
set +e
compose run --rm --no-deps --entrypoint /bench/bin/kv.test \
  -e GIZCLAW_TEST_TIKV_PD=pd:2379 benchmark \
  -test.v -test.run '^TestTiKVDuplicateReads$' -test.timeout=3m >"$evidence/test.log" 2>&1
status=$?
set -e
if ! collect; then status=1; fi
for service in voter-a voter-b voter-c; do
  container="$(compose ps -a -q "$service")"
  state="$(docker inspect --format '{{.State.Running}} {{.RestartCount}}' "$container")"
  printf '%s %s\n' "$service" "$state" >>"$evidence/voter-health.txt"
  if [ "$state" != 'true 0' ]; then status=1; fi
done
if grep -E '\[FATAL\]|row\.write\.is_none' "$evidence/cluster.log" >"$evidence/fatal.log"; then status=1; fi
cleanup
trap - EXIT
cat "$evidence/test.log" "$evidence/voter-health.txt"
if [ "$status" -ne 0 ]; then tail -80 "$evidence/cluster.log"; fi
printf 'exit_code=%s\nevidence=%s\n' "$status" "$evidence"
exit "$status"
