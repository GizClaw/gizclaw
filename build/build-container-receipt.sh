#!/usr/bin/env bash
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 METADATA_DIR ASSET_DIR TAG SOURCE_COMMIT" >&2; exit 2; }
metadata_dir="$1" asset_dir="$2" tag="$3" commit="$4"
receipt="$asset_dir/container-image.json"
[[ ! -e "$receipt" && ! -L "$receipt" ]]
parts=()
for component in mem0 runtime; do
  for arch in amd64 arm64; do
    part="$metadata_dir/container-${component}-${arch}.json"
    [[ -f "$part" && ! -L "$part" && -s "$part" ]]
    parts+=("$part")
  done
done
jq -s --arg tag "$tag" --arg commit "$commit" \
  '{schema_version:2,tag:$tag,version:($tag | ltrimstr("v")),source_commit:$commit,images:.}' \
  "${parts[@]}" >"$receipt"
"$(dirname "${BASH_SOURCE[0]}")/check-container-receipt.sh" "$receipt" "$tag" "$commit" "$asset_dir"
