#!/usr/bin/env bash
# Pinned image locations shared by the local Docker E2E entrypoints.
gizclaw_images_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

gizclaw_image_sources() {
  cat <<'SOURCES'
ubuntu docker.io/library/ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3
python docker.io/library/python:3.12.11-slim-bookworm@sha256:519591d6871b7bc437060736b9f7456b8731f1499a57e22e6c285135ae657bf7
redis docker.io/library/redis:8.4.6-alpine@sha256:12da49daa000c2be4d55118574f889c8cc298140ecf343d04f345c818f227823
livekit docker.io/livekit/livekit-server:v1.13.6@sha256:e37d68f172556d02aa77968b9fc55ef481468c0315fa38e4fa6c56ce72e3a815
coturn docker.io/coturn/coturn:4.7.0-r0@sha256:99bf5bf6ab1c119862d0c3d2dfb2bbf805a86a131492cab18c148be64ae7d978
pgvector docker.io/pgvector/pgvector:pg17@sha256:cf134a767f474095eeba57e0117be8e568e011a63f33fbf252f14c9b760f8e6f
netshoot docker.io/nicolaka/netshoot:v0.13@sha256:a20c2531bf35436ed3766cd6cfe89d352b050ccc4d7005ce6400adf97503da1b
postgres docker.io/library/postgres:17-alpine@sha256:b0f9560a2de083e2cc7382e75f808c7381a32852a7ec49117deedb300e552b24
SOURCES
}

gizclaw_image_source() {
  gizclaw_image_sources | awk -v name="$1" '$1 == name { print $2; found = 1; exit } END { if (!found) exit 1 }'
}

gizclaw_image_reference() {
  local source key arch="$2"
  source="$(gizclaw_image_source "$1")" || { echo "unknown E2E image: $1" >&2; return 2; }
  # This hashes the source string to locate its CDN directory, not archive bytes.
  if command -v sha256sum >/dev/null 2>&1; then
    key="$(printf '%s' "$source" | sha256sum | awk '{print $1}')" || return
  else
    key="$(printf '%s' "$source" | shasum -a 256 | awk '{print $1}')" || return
  fi
  printf 'gizclaw-tos/cache:%s-%s\n' "$key" "$arch"
}

gizclaw_image_env() {
  local arch="${1:-}" name reference variable
  if [[ -z "$arch" ]]; then
    arch="$(docker info --format '{{.Architecture}}')" || return
    case "$arch" in aarch64) arch=arm64 ;; x86_64) arch=amd64 ;; esac
  fi
  [[ "$arch" == amd64 || "$arch" == arm64 ]] || { echo "unsupported E2E Docker architecture: $arch" >&2; return 2; }
  while IFS= read -r name; do
    reference="$(gizclaw_image_reference "$name" "$arch")" || return
    variable="$(printf '%s' "$name" | tr '[:lower:]-' '[:upper:]_')"
    export "GIZCLAW_E2E_${variable}_IMAGE=$reference"
  done < <(gizclaw_image_sources | awk '{print $1}')
  export GIZCLAW_DOCKER_ARCH="$arch"
}

gizclaw_load_image() (
  set -euo pipefail
  name="$1" arch="$2"
  reference="$(gizclaw_image_reference "$name" "$arch")"
  if docker image inspect "$reference" >/dev/null 2>&1; then
    printf '%s\n' "$reference"
    exit 0
  fi
  key="${reference#gizclaw-tos/cache:}"
  key="${key%-"$arch"}"
  directory="${GIZCLAW_CONTAINER_IMAGE_CACHE_DIR:-$gizclaw_images_root/.tmp/container-images}/$key/$arch"
  archive="$directory/image.tar.gz"
  temporary=""
  trap '[[ -z "$temporary" ]] || rm -f -- "$temporary"' EXIT
  if [[ ! -f "$archive" ]]; then
    [[ "${GIZCLAW_CONTAINER_CACHE_ONLY:-0}" != 1 ]] || { echo "required cached image is missing: $name/$arch" >&2; exit 1; }
    mkdir -p "$directory"
    temporary="$(mktemp "$directory/.download-XXXXXX")"
    url="https://static-volc.gizclaw.com/releases/GizClaw/deploy/container-images/v1/$key/$arch/image.tar.gz"
    echo "E2E image download: $url" >&2
    curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
      --retry 3 --connect-timeout 20 --max-time 900 --output "$temporary" "$url"
    mv "$temporary" "$archive"
    temporary=""
  fi
  docker load --input "$archive" >&2
  printf '%s\n' "$reference"
)

gizclaw_require_images() {
  local name reference variable
  [[ -n "${GIZCLAW_DOCKER_ARCH:-}" ]] || gizclaw_image_env || return
  for name in "$@"; do
    reference="$(gizclaw_load_image "$name" "$GIZCLAW_DOCKER_ARCH")" || return
    variable="$(printf '%s' "$name" | tr '[:lower:]-' '[:upper:]_')"
    export "GIZCLAW_E2E_${variable}_IMAGE=$reference"
  done
}

gizclaw_require_local_builder() {
  local driver
  driver="$(docker buildx inspect | awk '/^Driver:/ {print $2}')" || return
  [[ "$driver" == docker ]] || { echo "E2E builds require the Docker driver sharing the loaded local images: $driver" >&2; return 2; }
  export DOCKER_BUILDKIT=1 COMPOSE_BAKE=false
}

gizclaw_compose_require_images() {
  local images name reference
  [[ -n "${GIZCLAW_DOCKER_ARCH:-}" ]] || gizclaw_image_env || return
  images="$(docker compose "$@" config --images)" || return
  while IFS= read -r name; do
    reference="$(gizclaw_image_reference "$name" "$GIZCLAW_DOCKER_ARCH")" || return
    if printf '%s\n' "$images" | grep -Fxq -- "$reference"; then
      gizclaw_require_images "$name" || return
    fi
  done < <(gizclaw_image_sources | awk '{print $1}')
}

gizclaw_prepare_mem0_base() {
  local platform="$1" image="$2" flavor="${3:-}"
  gizclaw_image_env "${platform#linux/}" || return
  gizclaw_require_images python || return
  gizclaw_require_local_builder || return
  TARGET=base PLATFORM="$platform" BASE_IMAGE="$image" PYTHON_BASE_IMAGE="$GIZCLAW_E2E_PYTHON_IMAGE" \
    "$gizclaw_images_root/build/build-mem0.sh" ${flavor:+"$flavor"}
}
