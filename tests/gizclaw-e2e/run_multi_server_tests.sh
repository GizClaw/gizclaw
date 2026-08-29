#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker/docker-compose.multi-server.yaml"
project="gizclaw-multi-server-${RANDOM}-$$"
bin_dir="$script_dir/testdata/bin"
server_bin="$bin_dir/gizclaw-linux"
test_bin="$bin_dir/multiserver-e2e-linux"

if [[ -z "${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-}" ]]; then
  case "$(docker info --format '{{.Architecture}}')" in
    aarch64 | arm64)
      if docker image inspect gizclaw-go:linux-arm64-cn-base-go1.26.4 >/dev/null 2>&1; then
        GIZCLAW_E2E_DOCKER_BASE_IMAGE="gizclaw-go:linux-arm64-cn-base-go1.26.4"
      else
        GIZCLAW_E2E_DOCKER_BASE_IMAGE="gizclaw-go:linux-arm64-cn-base"
      fi
      ;;
    *) GIZCLAW_E2E_DOCKER_BASE_IMAGE="gizclaw-go:linux-amd64-cn-base" ;;
  esac
  export GIZCLAW_E2E_DOCKER_BASE_IMAGE
fi

cleanup() {
  status=$?
  if (( status != 0 )); then
    docker compose -p "$project" -f "$compose_file" ps >&2 || true
    docker compose -p "$project" -f "$compose_file" logs --no-color --tail 800 \
      | grep -E 'peer stream lifecycle|gateway logical session|level=(WARN|ERROR)' >&2 || true
  fi
  docker compose -p "$project" -f "$compose_file" down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ "${GIZCLAW_E2E_KEEP_BINARIES:-}" != 1 ]]; then
    rm -f "$server_bin" "$test_bin"
  fi
}
trap cleanup EXIT

mkdir -p "$bin_dir"
module_cache="$(go env GOMODCACHE)"
build_cache="$(go env GOCACHE)"
if [[ "${GIZCLAW_E2E_REUSE_BINARIES:-}" != 1 || ! -x "$server_bin" || ! -x "$test_bin" ]]; then
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_root:/src" \
    -v "$module_cache:/root/go/pkg/mod" \
    -v "$build_cache:/root/.cache/go-build" \
    "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" \
    -lc 'cd /src && go build -o tests/gizclaw-e2e/testdata/bin/gizclaw-linux ./cmd/gizclaw && go test -c -tags gizclaw_e2e -o tests/gizclaw-e2e/testdata/bin/multiserver-e2e-linux ./tests/gizclaw-e2e/go/multiserver'
fi

docker compose -p "$project" -f "$compose_file" up --build --wait redis server-a server-b edge-a edge-b
docker compose -p "$project" -f "$compose_file" run --rm tests \
  -test.run "${GIZCLAW_E2E_TEST_RUN:-.}"

if docker compose -p "$project" -f "$compose_file" exec -T redis \
  redis-cli --scan --pattern '*runs*' | grep -q .; then
  echo "shared Redis unexpectedly contains PeerRun keys" >&2
  exit 1
fi

for edge in edge-a edge-b; do
  if docker compose -p "$project" -f "$compose_file" exec -T "$edge" env \
    | grep -Eq '^(REDIS_URL|REDIS_DSN|GIZCLAW_E2E_REDIS_DSN)='; then
    echo "$edge unexpectedly received Redis credentials" >&2
    exit 1
  fi
done

if docker compose -p "$project" -f "$compose_file" logs --no-color edge-a edge-b \
  | grep -Fq 'redis://redis:6379/0'; then
  echo "Edge diagnostics exposed the Redis DSN" >&2
  exit 1
fi
