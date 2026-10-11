#!/usr/bin/env bash

# Shared by Compose setup and the Linux CGO builder. The selected image is
# returned in base_image.
build_gizclaw_e2e_base() {
  local repo_root="$1" docker_platform="$2"
  local mapping variable argument value canonical dockerfile
  local platform_slug="${docker_platform//\//-}"
  local variant=cn
  local build_args=()
  # shellcheck source=../../docker-images/common.sh
  # shellcheck disable=SC1091
  source "$repo_root/tests/docker-images/common.sh"
  canonical="$(python3 "$repo_root/tests/docker-images/load.py" source ubuntu)" || return
  case "${GIZCLAW_E2E_DOCKER_BASE_FROM:-}" in
    "" | ubuntu:24.04 | docker.io/library/ubuntu:24.04 | docker.1ms.run/library/ubuntu:24.04 | "$canonical") ;;
    *) echo "E2E Ubuntu must use the pinned catalog source; declare other bases in the TOS image catalog" >&2; return 2 ;;
  esac
  gizclaw_image_env "${docker_platform#linux/}" || return
  gizclaw_require_images ubuntu || return
  gizclaw_require_local_builder || return
  build_args+=(--build-arg "BASE_IMAGE=$GIZCLAW_E2E_UBUNTU_IMAGE")
  for mapping in \
    APT_MIRROR:APT_MIRROR \
    APT_PORTS_MIRROR:APT_PORTS_MIRROR \
    GO_MIRROR:GO_MIRROR \
    NODE_MIRROR:NODE_MIRROR \
    GOPROXY:GOPROXY \
    GOSUMDB:GOSUMDB \
    NPM_REGISTRY:NPM_REGISTRY; do
    variable="GIZCLAW_E2E_${mapping%%:*}"
    argument="${mapping#*:}"
    value="${!variable:-}"
    if [[ -n "$value" ]]; then
      build_args+=(--build-arg "$argument=$value")
      variant=custom
    fi
  done
  base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:${platform_slug}-${variant}-base}"
  dockerfile="${GIZCLAW_E2E_DOCKER_BASE_DOCKERFILE:-$repo_root/build/gizclaw/Dockerfile.cn.base}"
  gizclaw_dockerfile_require_bases "$dockerfile" "$repo_root/build" "$GIZCLAW_E2E_UBUNTU_IMAGE" || return
  # Evaluate the current Dockerfile and mirror arguments each time; layer caches apply.
  echo "==> build e2e Docker base $base_image for $docker_platform"
  docker build --pull=false --platform="$docker_platform" \
    ${build_args[@]+"${build_args[@]}"} \
    -f "$dockerfile" -t "$base_image" "$repo_root/build"
}
