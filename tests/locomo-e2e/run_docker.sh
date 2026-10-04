#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker-compose.yaml"
project_name="${GIZCLAW_LOCOMO_E2E_DOCKER_PROJECT:-gizclaw-locomo}"
mem0_port="${GIZCLAW_LOCOMO_E2E_MEM0_PORT:-18000}"
mem0_pgvector_port="${GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_PORT:-18001}"
group="${1:-all}"
test_timeout="${GIZCLAW_LOCOMO_E2E_TEST_TIMEOUT:-60m}"

case "$group" in
  mem0)
    services=(mem0)
    test_pattern='^TestLoCoMoMem0SelfHosted$'
    ;;
  mem0-pgvector)
    services=(mem0_pgvector)
    test_pattern='^TestLoCoMoMem0SelfHostedPGVector$'
    ;;
  all)
    services=(mem0 mem0_pgvector)
    test_pattern='^TestLoCoMoMem0SelfHosted'
    ;;
  *)
    echo "usage: $0 [all|mem0|mem0-pgvector]" >&2
    exit 2
    ;;
esac

{
  required=(
    GIZCLAW_LOCOMO_E2E_MODEL_API_KEY
    GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY
  )
  for name in "${required[@]}"; do
    if [[ -z "${!name:-}" ]]; then
      echo "$name is required for self-hosted Mem0" >&2
      exit 2
    fi
  done
  if [[ -z "${GIZCLAW_LOCOMO_E2E_MEM0_LLM_BASE_URL:-${GIZCLAW_LOCOMO_E2E_MODEL_BASE_URL:-}}" ]]; then
    echo "GIZCLAW_LOCOMO_E2E_MEM0_LLM_BASE_URL or GIZCLAW_LOCOMO_E2E_MODEL_BASE_URL is required for self-hosted Mem0" >&2
    exit 2
  fi
}

cleanup() {
  docker compose --project-name "$project_name" --file "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT

{
  platform="$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}')"
  base_flavor=()
  suffix=base
  if [[ "${GIZCLAW_LOCOMO_E2E_MEM0_BASE_FLAVOR:-}" == cn ]]; then
    base_flavor=(cn)
    suffix=cn-base
  elif [[ -n "${GIZCLAW_LOCOMO_E2E_MEM0_BASE_FLAVOR:-}" ]]; then
    echo "GIZCLAW_LOCOMO_E2E_MEM0_BASE_FLAVOR must be cn or empty" >&2
    exit 2
  fi
  export GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE="${GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE:-gizclaw-mem0:${platform//\//-}-$suffix}"
  TARGET=base PLATFORM="$platform" BASE_IMAGE="$GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE" \
    "$repo_root/build/build-mem0.sh" ${base_flavor[@]+"${base_flavor[@]}"}
}

docker compose --project-name "$project_name" --file "$compose_file" up --detach --build --wait "${services[@]}"
export GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL="http://127.0.0.1:${mem0_port}"
export GIZCLAW_LOCOMO_E2E_MEM0_PGVECTOR_URL="http://127.0.0.1:${mem0_pgvector_port}"

cd "$repo_root"
go test -count=1 -timeout "$test_timeout" -v -tags gizclaw_locomo_e2e \
  -run "$test_pattern" ./tests/locomo-e2e
