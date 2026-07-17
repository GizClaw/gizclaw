#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
default_env="$e2e_dir/testdata/docker/current.env"
env_path="${GIZCLAW_E2E_DOCKER_ENV:-$default_env}"

if [[ -f "$env_path" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_path"
  set +a
fi

project="${GIZCLAW_E2E_DOCKER_PROJECT:-}"
compose_file="${GIZCLAW_E2E_DOCKER_COMPOSE_FILE:-$e2e_dir/docker/docker-compose.yaml}"

if [[ -z "$project" ]]; then
  echo "missing GIZCLAW_E2E_DOCKER_PROJECT; run docker-compose-up.sh first or set GIZCLAW_E2E_DOCKER_ENV" >&2
  exit 2
fi

docker compose -p "$project" -f "$compose_file" down -v --rmi local "$@"

state_dir="$e2e_dir/testdata/docker/$project"
rm -rf "$state_dir"
if [[ "$env_path" == "$default_env" ]]; then
  rm -f "$default_env"
fi
