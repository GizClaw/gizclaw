#!/usr/bin/env bash
# Shared image preflight. Reference calculation is offline; loading uses only TOS.
gizclaw_images_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

gizclaw_image_env() {
  local arch="${1:-}" references name reference
  if [[ -z "$arch" ]]; then
    arch="$(docker info --format '{{.Architecture}}')" || return
    case "$arch" in aarch64) arch=arm64 ;; x86_64) arch=amd64 ;; esac
  fi
  [[ "$arch" == amd64 || "$arch" == arm64 ]] || { echo "unsupported E2E Docker architecture: $arch" >&2; return 2; }
  references="$(python3 "$gizclaw_images_root/tests/docker-images/load.py" references "$arch")" || return
  while IFS=$'\t' read -r name reference; do export "$name=$reference"; done <<<"$references"
  export GIZCLAW_DOCKER_ARCH="$arch"
}

gizclaw_require_images() {
  local name reference variable
  [[ -n "${GIZCLAW_DOCKER_ARCH:-}" ]] || gizclaw_image_env || return
  for name in "$@"; do
    reference="$(bash "$gizclaw_images_root/tests/docker-images/load.sh" "$name" "$GIZCLAW_DOCKER_ARCH")" || return
    variable="${name//-/_}"
    variable="$(printf '%s' "$variable" | tr '[:lower:]' '[:upper:]')"
    export "GIZCLAW_E2E_${variable}_IMAGE=$reference"
  done
}

gizclaw_require_local_builder() {
  local driver
  driver="$(docker buildx inspect | awk '/^Driver:/ {print $2}')" || return
  [[ "$driver" == docker ]] || { echo "E2E builds require the Docker driver sharing the preloaded local image store: $driver" >&2; return 2; }
  export DOCKER_BUILDKIT=1 COMPOSE_BAKE=false
}

gizclaw_dockerfile_require_bases() {
  local dockerfile="$1" context="$2" image="$3"
  python3 "$gizclaw_images_root/tests/docker-images/load.py" dockerfile "$dockerfile" "$GIZCLAW_DOCKER_ARCH" \
    --context "$context" --base-image "$image"
}

gizclaw_compose_require_images() {
  local dependencies name
  [[ -n "${GIZCLAW_DOCKER_ARCH:-}" ]] || gizclaw_image_env || return
  dependencies="$(docker compose "$@" config --format json | python3 "$gizclaw_images_root/tests/docker-images/load.py" compose "$GIZCLAW_DOCKER_ARCH")" || return
  while IFS= read -r name; do
    [[ -z "$name" ]] || gizclaw_require_images "$name" || return
  done <<<"$dependencies"
}

gizclaw_register_built_image() {
  local image="$1" platform
  platform="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image")" || return
  [[ "$platform" == "linux/$GIZCLAW_DOCKER_ARCH" ]] || { echo "current-source image platform mismatch" >&2; return 1; }
  export GIZCLAW_E2E_BUILT_IMAGES="${GIZCLAW_E2E_BUILT_IMAGES:+$GIZCLAW_E2E_BUILT_IMAGES$'\n'}$image"
}

gizclaw_prepare_mem0_base() {
  local platform="$1" image="$2" flavor="${3:-}" dockerfile
  gizclaw_image_env "${platform#linux/}" || return
  gizclaw_require_images python || return
  gizclaw_require_local_builder || return
  dockerfile="$gizclaw_images_root/build/mem0/Dockerfile.base"
  [[ "$flavor" != cn ]] || dockerfile="$gizclaw_images_root/build/mem0/Dockerfile.cn.base"
  gizclaw_dockerfile_require_bases "$dockerfile" "$gizclaw_images_root" "$GIZCLAW_E2E_PYTHON_IMAGE" || return
  TARGET=base PLATFORM="$platform" BASE_IMAGE="$image" PYTHON_BASE_IMAGE="$GIZCLAW_E2E_PYTHON_IMAGE" \
    "$gizclaw_images_root/build/build-mem0.sh" ${flavor:+"$flavor"}
}
