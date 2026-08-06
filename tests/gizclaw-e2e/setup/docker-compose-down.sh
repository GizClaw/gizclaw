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
compose_args=(-f "$compose_file")
if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
  compose_args+=(-f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")
fi

if [[ -z "$project" ]]; then
  echo "missing GIZCLAW_E2E_DOCKER_PROJECT; run docker-compose-up.sh first or set GIZCLAW_E2E_DOCKER_ENV" >&2
  exit 2
fi
if [[ ! "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
  echo "invalid GIZCLAW_E2E_DOCKER_PROJECT: $project" >&2
  exit 2
fi

down_args=(down -v)
if [[ "${GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES:-}" != "1" ]]; then
  down_args+=(--rmi local)
fi
docker compose -p "$project" "${compose_args[@]}" "${down_args[@]}" "$@"

cleanup_project_resources() {
  local container_id network_id volume_name

  while IFS= read -r container_id; do
    if [[ -n "$container_id" ]]; then
      docker container rm --force "$container_id"
    fi
  done < <(docker ps -aq --filter "label=com.docker.compose.project=$project")

  while IFS= read -r network_id; do
    if [[ -n "$network_id" ]]; then
      docker network rm "$network_id"
    fi
  done < <(docker network ls -q --filter "label=com.docker.compose.project=$project")

  while IFS= read -r volume_name; do
    if [[ -n "$volume_name" ]]; then
      docker volume rm "$volume_name"
    fi
  done < <(docker volume ls -q --filter "label=com.docker.compose.project=$project")
}

# A canceled `docker compose up` plugin can finish creating a resource after
# `docker compose down` returns. Require two consecutive empty observations so
# cleanup completion means that this exact project has actually settled.
for _ in 1 2 3 4 5; do
  cleanup_project_resources
  sleep 1
  if [[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ]] &&
    [[ -z "$(docker network ls -q --filter "label=com.docker.compose.project=$project")" ]] &&
    [[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=$project")" ]]; then
    sleep 1
    if [[ -z "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ]] &&
      [[ -z "$(docker network ls -q --filter "label=com.docker.compose.project=$project")" ]] &&
      [[ -z "$(docker volume ls -q --filter "label=com.docker.compose.project=$project")" ]]; then
      break
    fi
  fi
done

if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ]] ||
  [[ -n "$(docker network ls -q --filter "label=com.docker.compose.project=$project")" ]] ||
  [[ -n "$(docker volume ls -q --filter "label=com.docker.compose.project=$project")" ]]; then
  echo "Docker resources remain after cleanup: project=$project" >&2
  exit 1
fi

if [[ "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" == *"docker-compose.gateway-relay.yaml" ]]; then
  rm -f \
    "$e2e_dir/testdata/edge-workspace/config.yaml" \
    "$e2e_dir/testdata/edge-workspace/gizclaw-edge.log" \
    "$e2e_dir/testdata/edge-workspace/gizclaw-edge.pid"
fi

state_dir="$e2e_dir/testdata/docker/$project"
rm -rf "$state_dir"
if [[ "$env_path" == "$default_env" ]]; then
  rm -f "$default_env"
fi
