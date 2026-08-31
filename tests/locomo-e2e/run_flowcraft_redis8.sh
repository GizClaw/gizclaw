#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker-compose.yaml"
project_name="${GIZCLAW_LOCOMO_E2E_DOCKER_PROJECT:-gizclaw-locomo-redis8}"
redis_port="${GIZCLAW_LOCOMO_E2E_REDIS8_PORT:-16380}"

cleanup() {
  docker compose --project-name "$project_name" --file "$compose_file" down --volumes --remove-orphans
}
trap cleanup EXIT

docker compose --project-name "$project_name" --file "$compose_file" up --detach --wait redis8
export GIZCLAW_LOCOMO_E2E_FLOWCRAFT_REDIS8_URL="redis://127.0.0.1:${redis_port}/0"

cd "$repo_root"
go test -count=1 -timeout 30m -v -tags gizclaw_locomo_e2e \
  -run '^TestLoCoMoFlowcraft' ./tests/locomo-e2e
