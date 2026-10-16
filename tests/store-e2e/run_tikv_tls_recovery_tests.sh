#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/.testbench/tikv-tls-$(date +%Y%m%d-%H%M%S)-$$}"
mkdir -p "$evidence/bin"
evidence="$(cd "$evidence" && pwd)"
fixture_dir="$(mktemp -d "$evidence/fixtures.XXXXXX")"
export TIKV_TLS_BIN_DIR="$evidence/bin" TIKV_TLS_FIXTURE_DIR="$fixture_dir"
project="gizclaw-tikv-tls-$(date +%s)-$$"
printf '%s\n' "$project" >"$evidence/project.txt"
compose_file="$repo_root/tests/store-e2e/docker-compose.tikv-tls-recovery.yaml"
compose() { docker compose -p "$project" -f "$compose_file" "$@"; }
cleanup() {
  local cleanup_status=0
  compose logs --no-color >"$evidence/cluster.log" 2>&1 || true
  compose down --volumes --remove-orphans >"$evidence/cleanup.log" 2>&1 || cleanup_status=$?
  rm -rf "$fixture_dir" || cleanup_status=$?
  return "$cleanup_status"
}
trap cleanup EXIT
case "$(docker info --format '{{.Architecture}}')" in
  arm64|aarch64) test_arch=arm64 ;;
  amd64|x86_64) test_arch=amd64 ;;
  *) echo 'Unsupported Docker architecture' >&2; exit 1 ;;
esac
GOOS=linux GOARCH="$test_arch" CGO_ENABLED=0 go build -o "$TIKV_TLS_BIN_DIR/probe" ./tests/store-e2e/tikv-tls-recovery
generator_image="$(compose config --format json | python3 -c 'import json,sys; print(json.load(sys.stdin)["services"]["probe"]["image"])')"
docker run --rm --user "$(id -u):$(id -g)" --entrypoint /test/probe \
  -v "$TIKV_TLS_BIN_DIR:/test:ro" -v "$TIKV_TLS_FIXTURE_DIR:/fixtures" \
  "$generator_image" certs /fixtures
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
git status --short >"$evidence/source-status.txt"
go version >"$evidence/go-version.txt"
compose pull pd-a pd-b probe tikv-a tikv-b >"$evidence/pull.log" 2>&1
compose up -d pd-a pd-b probe tikv-a tikv-b >"$evidence/startup.log" 2>&1
container="$(compose ps -a -q probe)"
test -n "$container"
status="$(docker wait "$container")"
cleanup
trap - EXIT
compose_status="${status:-1}"
case "$compose_status" in *[!0-9]*) echo 'Invalid fixture exit status' >&2; exit 1 ;; esac
printf 'exit_code=%s\nevidence=%s\n' "$compose_status" "$evidence"
exit "$compose_status"
