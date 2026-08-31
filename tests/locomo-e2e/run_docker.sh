#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker-compose.yaml"
project_name="${GIZCLAW_LOCOMO_E2E_DOCKER_PROJECT:-gizclaw-locomo}"
redis_port="${GIZCLAW_LOCOMO_E2E_REDIS8_PORT:-16380}"
mem0_port="${GIZCLAW_LOCOMO_E2E_MEM0_PORT:-18000}"
group="${1:-all}"

case "$group" in
  flowcraft)
    services=(redis8)
    test_pattern='^TestLoCoMoFlowcraft'
    ;;
  mem0)
    services=(mem0)
    test_pattern='^TestLoCoMoMem0SelfHosted$'
    ;;
  all)
    services=(redis8 mem0)
    test_pattern='^TestLoCoMo(Flowcraft|Mem0SelfHosted)'
    ;;
  *)
    echo "usage: $0 [all|flowcraft|mem0]" >&2
    exit 2
    ;;
esac

if [[ "$group" != "flowcraft" ]]; then
  required=(
    GIZCLAW_LOCOMO_E2E_MODEL_API_KEY
    GIZCLAW_LOCOMO_E2E_MODEL_BASE_URL
    GIZCLAW_LOCOMO_E2E_EMBEDDING_API_KEY
  )
  for name in "${required[@]}"; do
    if [[ -z "${!name:-}" ]]; then
      echo "$name is required for self-hosted Mem0" >&2
      exit 2
    fi
  done
fi

cleanup() {
  docker compose --project-name "$project_name" --file "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT

docker compose --project-name "$project_name" --file "$compose_file" up --detach --build --wait "${services[@]}"
export GIZCLAW_LOCOMO_E2E_FLOWCRAFT_REDIS8_URL="redis://127.0.0.1:${redis_port}/0"
export GIZCLAW_LOCOMO_E2E_MEM0_SELF_HOSTED_URL="http://127.0.0.1:${mem0_port}"

cd "$repo_root"
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run "$test_pattern" ./tests/locomo-e2e
