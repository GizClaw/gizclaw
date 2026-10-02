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
  *) echo "unsupported PLATFORM: $platform" >&2; exit 2 ;;
esac
platform_slug="${platform//\//-}"
case "$flavor" in
  "")
    base_dockerfile="$repo_root/build/mem0/Dockerfile.base"
    base_image="${BASE_IMAGE:-gizclaw-mem0:${platform_slug}-base}"
    ;;
  cn)
    base_dockerfile="$repo_root/build/mem0/Dockerfile.cn.base"
    base_image="${BASE_IMAGE:-gizclaw-mem0:${platform_slug}-cn-base}"
    ;;
  *) echo "usage: $0 [cn]" >&2; exit 2 ;;
esac
target="${TARGET:-runtime}"
case "$target" in
  base | test | runtime) ;;
  *) echo "TARGET must be base, test, or runtime" >&2; exit 2 ;;
esac
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
base_args=()
if [[ -n "${PYTHON_BASE_IMAGE:-}" ]]; then
  base_args+=(--build-arg "BASE_IMAGE=$PYTHON_BASE_IMAGE")
fi
if [[ -n "${PIP_INDEX_URL:-}" ]]; then
  if [[ "$flavor" != cn ]]; then
    echo "PIP_INDEX_URL override requires cn" >&2
    exit 2
  fi
  base_args+=(--build-arg "PIP_INDEX_URL=$PIP_INDEX_URL")
fi
docker build --platform "$platform" -f "$base_dockerfile" \
  ${base_args[@]+"${base_args[@]}"} -t "$base_image" "$repo_root"
if [[ "$target" == base ]]; then
  exit 0
fi
image="${IMAGE:-gizclaw-mem0:${platform_slug}}"
docker build --provenance=false --platform "$platform" --target "$target" \
  --build-arg "BASE_IMAGE=$base_image" --build-arg "VERSION=$build_version" \
  --build-arg "SOURCE_COMMIT=$build_commit" -f "$repo_root/build/mem0/Dockerfile" \
  -t "$image" "$repo_root"
