#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if (($# > 1)); then
  echo "usage: $0 [cn]" >&2
  exit 2
fi

flavor="${1:-}"
platform="${PLATFORM:-linux/amd64}"
case "$platform" in
  linux/amd64 | linux/arm64) ;;
  *)
    echo "unsupported PLATFORM: $platform" >&2
    exit 2
    ;;
esac

target_os="${platform%%/*}"
target_arch="${platform##*/}"
platform_slug="${target_os}-${target_arch}"
image="${IMAGE:-gizclaw-go:${platform_slug}-build}"
output="${OUTPUT:-${repo_root}/.tmp/deploy/gizclaw-${platform_slug}}"
build_version="${BUILD_VERSION:-dev}"
build_commit="${BUILD_COMMIT:-dev}"

if [[ "$build_version" != dev && ! "$build_version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "BUILD_VERSION must be dev or a stable SemVer without a leading v" >&2
  exit 2
fi
if [[ "$build_commit" != dev && ! "$build_commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "BUILD_COMMIT must be dev or a full lowercase source commit" >&2
  exit 2
fi

export DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}"

case "$flavor" in
  "")
    base_dockerfile="$repo_root/build/Dockerfile.base"
    base_image="${BASE_IMAGE:-gizclaw-go:${platform_slug}-base}"
    ;;
  cn)
    base_dockerfile="$repo_root/build/Dockerfile.cn.base"
    base_image="${BASE_IMAGE:-gizclaw-go:${platform_slug}-cn-base}"
    ;;
  *)
    echo "usage: $0 [cn]" >&2
    exit 2
    ;;
esac

mkdir -p "$(dirname "$output")"

docker build \
  --platform "$platform" \
  -f "$base_dockerfile" \
  -t "$base_image" \
  "$repo_root/build"

docker build \
  --platform "$platform" \
  --build-arg BASE_IMAGE="$base_image" \
  --build-arg TARGETOS="$target_os" \
  --build-arg TARGETARCH="$target_arch" \
  --build-arg BUILD_VERSION="$build_version" \
  --build-arg BUILD_COMMIT="$build_commit" \
  --target artifact \
  -f "$repo_root/build/Dockerfile" \
  -t "$image" \
  "$repo_root"

container="$(docker create --platform "$platform" "$image" /gizclaw)"
trap 'docker rm -f "$container" >/dev/null 2>&1 || true' EXIT

docker cp "$container:/gizclaw" "$output"
chmod 0755 "$output"

file "$output"
