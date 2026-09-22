#!/usr/bin/env bash

# Shared by Compose setup and the Linux CGO builder. The selected image is
# returned in base_image.
build_gizclaw_e2e_base() {
  local repo_root="$1" docker_platform="$2"
  local mapping variable argument value
  local platform_slug="${docker_platform//\//-}"
  local variant=cn
  local build_args=()
  for mapping in \
    DOCKER_BASE_FROM:BASE_IMAGE \
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
  # Always evaluate overridden builds so a cached tag cannot hide changed
  # sources. Docker's layer cache still applies. Keep the default CN tag intact.
  if ((${#build_args[@]} > 0)) || ! docker image inspect "$base_image" >/dev/null 2>&1; then
    echo "==> build e2e Docker base $base_image for $docker_platform"
    docker build --platform="$docker_platform" \
      ${build_args[@]+"${build_args[@]}"} \
      -f "$repo_root/build/Dockerfile.cn.base" -t "$base_image" "$repo_root/build"
  fi
}
