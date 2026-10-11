#!/usr/bin/env bash
# Load and exercise the actual delivered archive on a matching native runner.
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 ENTRY TAG SOURCE_COMMIT ASSET_DIR" >&2; exit 2; }
entry="$1" tag="$2" commit="$3" asset_dir="$4"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
python3 "$repo_root/build/container_archive.py" entry "$entry" "$tag" "$commit" "$asset_dir"
archive="$(jq -er .name "$entry")"
reference="$(jq -er .image_reference "$entry")"
arch="$(jq -er .architecture "$entry")"
docker load --input "$asset_dir/$archive"
image_id="$(docker image inspect --format '{{.Id}}' "$reference")"
# Classic Docker identifies the config; containerd may identify a manifest or its OCI index.
jq -e --arg id "$image_id" '$id == .config_digest or $id == .archive_manifest_digest or $id == .archive_index_digest' "$entry" >/dev/null
if [[ "$(jq -er .component "$entry")" == runtime ]]; then
  "$repo_root/build/check-runtime-image.sh" "$image_id" "${tag#v}" "$commit" "$arch" "$(jq -er .binary_sha256 "$entry")"
else
  "$repo_root/build/check-mem0-image.sh" "$image_id" "${tag#v}" "$commit" "$arch"
fi
