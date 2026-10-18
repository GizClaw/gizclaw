#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
evidence="${1:-$repo_root/.testbench/tikv-policy-$(date +%Y%m%d-%H%M%S)-$$}"
mkdir -p "$evidence/bin"
evidence="$(cd "$evidence" && pwd)"
export TIKV_E2E_BIN_DIR="$evidence/bin"
project="gizclaw-tikv-policy-$(date +%s)-$$"
compose() { docker compose -p "$project" -f tests/store-e2e/docker-compose.tikv-policy.yaml "$@"; }
cleanup() {
  compose ps -a --format json >"$evidence/topology.json" 2>&1 || true
  compose logs --no-color >"$evidence/cluster.log" 2>&1 || true
  compose down --volumes --remove-orphans >"$evidence/cleanup.log" 2>&1
}
trap cleanup EXIT
case "$(docker info --format '{{.Architecture}}')" in
  arm64|aarch64) arch=arm64 ;;
  amd64|x86_64) arch=amd64 ;;
  *) echo 'Unsupported Docker architecture' >&2; exit 2 ;;
esac
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o "$TIKV_E2E_BIN_DIR/replication-proxy" ./tests/store-e2e/tikv-replication-proxy
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go test -c -tags=store_e2e -o "$TIKV_E2E_BIN_DIR/store.test" ./tests/store-e2e
git rev-parse HEAD >"$evidence/source-revision.txt"
git diff HEAD >"$evidence/source.patch"
git status --short >"$evidence/source-status.txt"
printf '%s\n' "$project" >"$evidence/project.txt"
compose up -d pd leader gate learner gate2 learner2 >"$evidence/startup.log" 2>&1
set +e
compose run --rm --no-deps tests >"$evidence/test.log" 2>&1
status=$?
set -e
cleanup
trap - EXIT
cat "$evidence/test.log"
printf 'exit_code=%s\nevidence=%s\n' "$status" "$evidence"
exit "$status"
