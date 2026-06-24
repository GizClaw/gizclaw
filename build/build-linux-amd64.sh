#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if (($# > 1)); then
  echo "usage: $0 [cn]" >&2
  exit 2
fi

flavor="${1:-}"
platform="${PLATFORM:-linux/amd64}"
image="${IMAGE:-gizclaw-go:linux-amd64-build}"
output="${OUTPUT:-${repo_root}/.tmp/deploy/gizclaw-linux-amd64}"

export DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}"

case "$flavor" in
  "")
    base_dockerfile="$repo_root/build/Dockerfile.base"
    base_image="${BASE_IMAGE:-gizclaw-go:linux-amd64-base}"
    ;;
  cn)
    base_dockerfile="$repo_root/build/Dockerfile.cn.base"
    base_image="${BASE_IMAGE:-gizclaw-go:linux-amd64-cn-base}"
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
  --target artifact \
  -f "$repo_root/build/Dockerfile" \
  -t "$image" \
  "$repo_root"

container="$(docker create --platform "$platform" "$image" /gizclaw)"
trap 'docker rm -f "$container" >/dev/null 2>&1 || true' EXIT

docker cp "$container:/gizclaw" "$output"
chmod 0755 "$output"

file "$output"
