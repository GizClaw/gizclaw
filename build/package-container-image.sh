#!/usr/bin/env bash
# Export one local image; metadata parts are CI intermediates, not Release assets.
set -euo pipefail
[[ $# == 7 ]] || { echo "usage: $0 COMPONENT IMAGE ASSET_DIR METADATA_DIR TAG SOURCE_COMMIT ARCH" >&2; exit 2; }
component="$1" image="$2" asset_dir="$3" metadata_dir="$4" tag="$5" commit="$6" arch="$7"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$component" == runtime || "$component" == mem0 ]]
[[ "$arch" == amd64 || "$arch" == arm64 ]]
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$commit" =~ ^[0-9a-f]{40}$ ]]
mkdir -p "$asset_dir" "$metadata_dir"
archive="$asset_dir/gizclaw-${component}_${tag#v}_linux_${arch}.tar.gz"
entry="$metadata_dir/container-${component}-${arch}.json"
[[ ! -e "$archive" && ! -L "$archive" && ! -e "$entry" && ! -L "$entry" ]]
reference="gizclaw-${component}:${tag}-${arch}"
docker tag "$image" "$reference"
docker save "$reference" | gzip -n >"$archive"
image_id="$(docker image inspect --format '{{.Id}}' "$reference")"
python3 "$repo_root/build/container_archive.py" inspect "$archive" "$component" "$tag" "$commit" "$arch" "$image_id" >"$entry"
if [[ "$component" == runtime ]]; then
  binary="$(dpkg-deb --fsys-tarfile "$asset_dir/gizclaw_${tag#v}_${arch}.deb" | tar -xOf - ./usr/bin/gizclaw | sha256sum | cut -d ' ' -f1)"
  jq --arg binary "$binary" '. + {binary_sha256:$binary}' "$entry" >"$entry.tmp"
  mv "$entry.tmp" "$entry"
fi
python3 "$repo_root/build/container_archive.py" entry "$entry" "$tag" "$commit" "$asset_dir"
