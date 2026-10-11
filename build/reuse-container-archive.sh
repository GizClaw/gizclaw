#!/usr/bin/env bash
# Reuse immutable bytes from an exact-tag Release or draft; only absence exits 3.
set -euo pipefail
[[ $# == 6 ]] || { echo "usage: $0 COMPONENT ASSET_DIR METADATA_DIR TAG SOURCE_COMMIT ARCH" >&2; exit 2; }
component="$1" asset_dir="$2" metadata_dir="$3" tag="$4" commit="$5" arch="$6"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$component" == runtime || "$component" == mem0 ]]
[[ "$arch" == amd64 || "$arch" == arm64 ]]
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$commit" =~ ^[0-9a-f]{40}$ ]]
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
status=0
"$repo_root/build/find-release-by-tag.sh" "${GH_REPO:-GizClaw/gizclaw}" "$tag" >"$work/release.json" || status=$?
[[ "$status" != 3 ]] || exit 3
[[ "$status" == 0 ]] || exit 1
jq -e --arg tag "$tag" --arg commit "$commit" '
  .tag_name == $tag and .target_commitish == $commit and .prerelease == false and
  (.draft | type == "boolean") and (.assets | length == 19) and ([.assets[].name] | unique | length == 19)
' "$work/release.json" >/dev/null
download() {
  local name="$1" destination="$2" asset_id
  asset_id="$(jq -er --arg name "$name" '[.assets[] | select(.name == $name)] |
    if length == 1 then .[0].id | select(type == "number" and . > 0) else error("missing or duplicate asset") end' "$work/release.json")"
  gh api -H 'Accept: application/octet-stream' "repos/${GH_REPO:-GizClaw/gizclaw}/releases/assets/$asset_id" >"$destination"
}
download container-image.json "$work/container-image.json"
download release-manifest.json "$work/release-manifest.json"
download SHA256SUMS "$work/SHA256SUMS"
jq -e --arg tag "$tag" --arg commit "$commit" \
  --arg receipt_sha "$(sha256sum "$work/container-image.json" | cut -d ' ' -f1)" \
  --argjson receipt_size "$(wc -c <"$work/container-image.json" | tr -d ' ')" \
  --slurpfile receipt "$work/container-image.json" '
    .schema_version == 8 and .tag == $tag and .source_commit == $commit and .release_channel == "stable" and
    (.assets | length == 16) and ([.assets[].name] | unique | length == 16) and
    ([.assets[] | select(.kind == "container-archive")] == $receipt[0].images) and
    ([.assets[] | select(.name == "container-image.json") | {sha256,size}] == [{sha256:$receipt_sha,size:$receipt_size}])
  ' "$work/release-manifest.json" >/dev/null
archive="gizclaw-${component}_${tag#v}_linux_${arch}.tar.gz"
mkdir -p "$asset_dir" "$metadata_dir"
[[ ! -e "$asset_dir/$archive" && ! -L "$asset_dir/$archive" ]]
download "$archive" "$asset_dir/$archive"
for name in container-image.json release-manifest.json "$archive"; do
  file="$work/$name"
  [[ "$name" != "$archive" ]] || file="$asset_dir/$name"
  checksum="$(sha256sum "$file" | cut -d ' ' -f1)  $name"
  [[ "$(grep -Fxc -- "$checksum" "$work/SHA256SUMS")" == 1 ]]
done
"$repo_root/build/check-container-receipt.sh" "$work/container-image.json" "$tag" "$commit" "$asset_dir" "$component" "$arch"
jq --arg component "$component" --arg arch "$arch" \
  '.images[] | select(.component == $component and .architecture == $arch)' \
  "$work/container-image.json" >"$metadata_dir/container-${component}-${arch}.json"
