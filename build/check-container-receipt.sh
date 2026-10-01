#!/usr/bin/env bash
# Validate immutable registry references, including the packaged binary digests.
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 RECEIPT TAG SOURCE_COMMIT ASSET_DIR" >&2; exit 2; }
receipt="$1" tag="$2" source_commit="$3" asset_dir="$4"
[[ -f "$receipt" && ! -L "$receipt" && -s "$receipt" ]]
jq -e --arg tag "$tag" --arg commit "$source_commit" '
  keys == ["base_image","digest","image","platforms","reference","schema_version","source_commit","tag","version"] and
  .schema_version == 1 and .tag == $tag and .version == ($tag | ltrimstr("v")) and
  .source_commit == $commit and .image == "ghcr.io/gizclaw/gizclaw" and
  (.digest | test("^sha256:[0-9a-f]{64}$")) and .reference == (.image + "@" + .digest) and
  .base_image == "ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3" and
  [.platforms[] | {os,architecture}] == [{os:"linux",architecture:"amd64"},{os:"linux",architecture:"arm64"}] and
  all(.platforms[];
    keys == ["architecture","binary_sha256","digest","os"] and
    (.digest | test("^sha256:[0-9a-f]{64}$")) and (.binary_sha256 | test("^[0-9a-f]{64}$")))
' "$receipt" >/dev/null
for arch in amd64 arm64; do
  binary_digest="$(dpkg-deb --fsys-tarfile "$asset_dir/gizclaw_${tag#v}_${arch}.deb" | tar -xOf - ./usr/bin/gizclaw | sha256sum | cut -d ' ' -f1)"
  [[ "$binary_digest" == "$(jq -er --arg arch "$arch" '.platforms[] | select(.architecture == $arch) | .binary_sha256' "$receipt")" ]] || {
    echo "container executable digest differs from Debian package: $arch" >&2
    exit 1
  }
done
