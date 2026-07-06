#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
default_env="$e2e_dir/testdata/docker/current.env"
env_path="${GIZCLAW_E2E_DOCKER_ENV:-$default_env}"

if [[ ! -f "$env_path" ]]; then
  echo "missing Docker e2e env: $env_path" >&2
  echo "run: bash tests/gizclaw-e2e/setup/docker-compose-up.sh" >&2
  exit 2
fi

set -a
# shellcheck disable=SC1090
source "$env_path"
set +a

compose_file="${GIZCLAW_E2E_DOCKER_COMPOSE_FILE:-$e2e_dir/docker/docker-compose.yaml}"
project="${GIZCLAW_E2E_DOCKER_PROJECT:-}"
if [[ -z "$project" ]]; then
  echo "missing GIZCLAW_E2E_DOCKER_PROJECT in $env_path" >&2
  exit 2
fi

docker compose -p "$project" -f "$compose_file" exec -T server \
  /src/tests/gizclaw-e2e/docker/setup/apply_client_view.sh "$@"
