#!/usr/bin/env bash
# Create deterministic release metadata from a closed payload directory.

set -euo pipefail

asset_dir=
tag=
debian_version=
source_commit=
while (($# > 0)); do
  case "$1" in
    --asset-dir) asset_dir="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --debian-version) debian_version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -d "$asset_dir" && ! -L "$asset_dir" && -n "$tag" && -n "$debian_version" && -n "$source_commit" ]] || {
  echo "usage: $0 --asset-dir DIR --tag latest|vMAJOR.MINOR.PATCH --debian-version VERSION --source-commit SHA" >&2
  exit 2
}
release_channel=stable
go_module_version="$tag"
if [[ "$tag" == latest ]]; then
  release_channel=snapshot
  go_module_version=
  [[ "$debian_version" =~ ^0\.0\.0~main\.[0-9]+\+[0-9a-f]{12}$ ]] || {
    echo "invalid canonical main snapshot Debian version" >&2
    exit 2
  }
elif [[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  [[ "$debian_version" == "${tag#v}" ]] || { echo "SemVer tag and Debian version differ" >&2; exit 2; }
else
  echo "invalid release tag" >&2
  exit 2
fi
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
for command_name in dpkg-deb jq sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

expected=(
  "gizclaw_${debian_version}_amd64.deb"
  "gizclaw_${debian_version}_arm64.deb"
  gizclaw-darwin-amd64
  gizclaw-darwin-arm64
)
actual=()
while IFS= read -r name; do actual+=("$name"); done < <(find "$asset_dir" -mindepth 1 -maxdepth 1 -exec basename {} \; | LC_ALL=C sort)
expected_sorted="$(printf '%s\n' "${expected[@]}" | LC_ALL=C sort)"
actual_sorted="$(printf '%s\n' "${actual[@]}" | LC_ALL=C sort)"
[[ "$actual_sorted" == "$expected_sorted" ]] || {
  echo "formal payload inventory is incomplete or contains unexpected files" >&2
  diff -u <(printf '%s\n' "$expected_sorted") <(printf '%s\n' "$actual_sorted") >&2 || true
  exit 1
}

assets_json='[]'
while IFS= read -r name; do
  artifact="$asset_dir/$name"
  [[ -f "$artifact" && ! -L "$artifact" && -s "$artifact" ]] || { echo "invalid payload: $name" >&2; exit 1; }
  kind=executable
  os=darwin
  architecture="${name##*-}"
  extra='{}'
  if [[ "$name" == *.deb ]]; then
    kind=deb
    os=linux
    architecture="$(dpkg-deb --field "$artifact" Architecture)"
    package="$(dpkg-deb --field "$artifact" Package)"
    package_version="$(dpkg-deb --field "$artifact" Version)"
    package_source="$(dpkg-deb --field "$artifact" X-GizClaw-Source-Commit)"
    [[ "$package" == gizclaw && "$package_version" == "$debian_version" && "$package_source" == "$source_commit" ]] || {
      echo "Debian metadata does not match release identity: $name" >&2
      exit 1
    }
    extra="$(jq -cn --arg package "$package" --arg version "$package_version" --arg source_commit "$package_source" \
      '{package:$package,version:$version,installed_path:"/usr/bin/gizclaw",source_commit:$source_commit}')"
  else
    [[ -x "$artifact" ]] || { echo "Darwin payload is not executable: $name" >&2; exit 1; }
  fi
  [[ "$architecture" == amd64 || "$architecture" == arm64 ]] || { echo "invalid payload architecture: $name" >&2; exit 1; }
  size="$(wc -c <"$artifact" | tr -d ' ')"
  digest="$(sha256sum "$artifact" | awk '{print $1}')"
  entry="$(jq -cn \
    --arg name "$name" --arg kind "$kind" --arg os "$os" --arg architecture "$architecture" \
    --argjson size "$size" --arg sha256 "$digest" --argjson extra "$extra" \
    '{name:$name,kind:$kind,os:$os,architecture:$architecture,size:$size,sha256:$sha256} + $extra')"
  assets_json="$(jq -c --argjson entry "$entry" '. + [$entry]' <<<"$assets_json")"
done < <(printf '%s\n' "${expected[@]}" | LC_ALL=C sort)

manifest="$asset_dir/release-manifest.json"
checksums="$asset_dir/SHA256SUMS"
[[ ! -e "$manifest" && ! -L "$manifest" && ! -e "$checksums" && ! -L "$checksums" ]] || {
  echo "release metadata already exists" >&2
  exit 1
}
jq -n \
  --arg release_channel "$release_channel" \
  --arg tag "$tag" \
  --arg go_module_version "$go_module_version" \
  --arg debian_version "$debian_version" \
  --arg source_commit "$source_commit" \
  --argjson assets "$assets_json" '
    {
      schema_version: 2,
      repository: "GizClaw/gizclaw",
      go_module: "github.com/GizClaw/gizclaw-go",
      release_channel: $release_channel,
      tag: $tag,
      go_module_version: (if $go_module_version == "" then null else $go_module_version end),
      debian_version: $debian_version,
      source_commit: $source_commit,
      workflow: ".github/workflows/release.yml",
      assets: $assets
    }
  ' >"$manifest"

{
  while IFS= read -r name; do
    printf '%s  %s\n' "$(sha256sum "$asset_dir/$name" | awk '{print $1}')" "$name"
  done < <(printf '%s\n' "${expected[@]}" release-manifest.json | LC_ALL=C sort)
} >"$checksums"

printf '%s\n' "built release metadata for $tag"
