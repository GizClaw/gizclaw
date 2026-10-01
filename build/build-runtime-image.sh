#!/usr/bin/env bash
# Package a previously verified Debian executable without rebuilding it.
set -euo pipefail
[[ $# == 5 ]] || { echo "usage: $0 PACKAGE VERSION SOURCE_COMMIT SOURCE_EPOCH ARCH" >&2; exit 2; }
package="$1" version="$2" source_commit="$3" source_epoch="$4" arch="$5"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]
[[ "$source_commit" =~ ^[0-9a-f]{40}$ && "$source_epoch" =~ ^[0-9]+$ ]]
[[ "$arch" == amd64 || "$arch" == arm64 ]]
[[ -f "$package" && ! -L "$package" && -s "$package" ]]
[[ "$(dpkg-deb -f "$package" Package)" == gizclaw ]]
[[ "$(dpkg-deb -f "$package" Version)" == "$version" ]]
[[ "$(dpkg-deb -f "$package" X-GizClaw-Source-Commit)" == "$source_commit" ]]
[[ "$(dpkg-deb -f "$package" Architecture)" == "$arch" ]]
context="$(mktemp -d)"
trap 'rm -rf "$context"' EXIT
cp "$package" "$context/gizclaw.deb"
cp "$repo_root/LICENSE" "$context/LICENSE"
cp "$repo_root/build/runtime-entrypoint.sh" "$context/runtime-entrypoint.sh"
created="$(date -u -d "@$source_epoch" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r "$source_epoch" +%Y-%m-%dT%H:%M:%SZ)"
docker build --provenance=false --platform "linux/$arch" -f "$repo_root/build/Dockerfile.runtime" \
  --build-arg VERSION="$version" --build-arg SOURCE_COMMIT="$source_commit" \
  --build-arg CREATED="$created" -t "gizclaw-runtime:${source_commit}-${arch}" "$context"
