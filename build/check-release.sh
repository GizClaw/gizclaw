#!/usr/bin/env bash
# Validate an exact mutable snapshot or protected formal-release inventory.

set -euo pipefail

requested_mode="${1:-}"
mode="$requested_mode"
asset_dir="${2:-}"
tag="${3:-}"
source_commit="${4:-}"
release_json="${5:-}"
case "$requested_mode" in
  snapshot)
    mode=release
    version="$tag"
    tag=latest
    release_channel=snapshot
    go_module_version=
    ;;
  semver)
    mode=release
    release_channel=stable
    go_module_version="$tag"
    ;;
  draft | published)
    mode=release
    release_channel=stable
    go_module_version="$tag"
    [[ -f "$release_json" && ! -L "$release_json" ]] || { echo "remote Release JSON must be a regular file" >&2; exit 2; }
    ;;
  *)
    echo "usage: $0 snapshot DIR DEBIAN_VERSION SOURCE_COMMIT | $0 semver DIR TAG SOURCE_COMMIT | $0 draft|published DIR TAG SOURCE_COMMIT RELEASE_JSON" >&2
    exit 2
    ;;
esac
[[ "$mode" == release ]] || {
  echo "invalid release mode" >&2
  exit 2
}
[[ -d "$asset_dir" && ! -L "$asset_dir" ]] || { echo "asset directory must be regular" >&2; exit 2; }
for command_name in cmp dpkg-deb jq sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

assert_inventory() {
  local expected_names="$1" actual_names
  actual_names="$(find "$asset_dir" -mindepth 1 -maxdepth 1 -exec basename {} \; | LC_ALL=C sort)"
  [[ "$actual_names" == "$expected_names" ]] || {
    echo "$mode inventory is incomplete or contains unexpected files" >&2
    printf 'expected:\n%s\nactual:\n%s\n' "$expected_names" "$actual_names" >&2
    return 1
  }
  while IFS= read -r name; do
    [[ -f "$asset_dir/$name" && ! -L "$asset_dir/$name" && -s "$asset_dir/$name" ]] || {
      echo "asset is not a regular non-empty file: $name" >&2
      return 1
    }
  done <<<"$expected_names"
}

if [[ "$requested_mode" == snapshot ]]; then
  [[ "$version" =~ ^0\.0\.0~main\.[0-9]+\+[0-9a-f]{12}$ ]] || { echo "invalid canonical main snapshot Debian version" >&2; exit 2; }
else
  [[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid canonical SemVer tag" >&2; exit 2; }
  version="${tag#v}"
fi
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
release_expected="$(printf '%s\n' \
  SHA256SUMS \
  gizclaw-darwin-amd64 \
  gizclaw-darwin-arm64 \
  "gizclaw_${version}_amd64.deb" \
  "gizclaw_${version}_arm64.deb" \
  release-manifest.json | LC_ALL=C sort)"
assert_inventory "$release_expected"

manifest="$asset_dir/release-manifest.json"
jq -e \
  --arg release_channel "$release_channel" \
  --arg tag "$tag" --arg go_module_version "$go_module_version" \
  --arg version "$version" --arg source_commit "$source_commit" '
  keys == ["assets","debian_version","go_module","go_module_version","release_channel","repository","schema_version","source_commit","tag","workflow"] and
  .schema_version == 2 and
  .repository == "GizClaw/gizclaw" and
  .go_module == "github.com/GizClaw/gizclaw-go" and
  .release_channel == $release_channel and
  .tag == $tag and
  .go_module_version == (if $go_module_version == "" then null else $go_module_version end) and
  .debian_version == $version and
  .source_commit == $source_commit and .workflow == ".github/workflows/release.yml" and
  (.assets | length == 4) and
  ([.assets[].name] == ([.assets[].name] | sort)) and
  ([.assets[].name] | unique | length == 4) and
  ([.assets[] | {name,kind,os,architecture}] == [
    {name:"gizclaw-darwin-amd64",kind:"executable",os:"darwin",architecture:"amd64"},
    {name:"gizclaw-darwin-arm64",kind:"executable",os:"darwin",architecture:"arm64"},
    {name:("gizclaw_" + $version + "_amd64.deb"),kind:"deb",os:"linux",architecture:"amd64"},
    {name:("gizclaw_" + $version + "_arm64.deb"),kind:"deb",os:"linux",architecture:"arm64"}
  ]) and
  all(.assets[];
    (keys | all(. == "architecture" or . == "installed_path" or . == "kind" or . == "name" or . == "os" or . == "package" or . == "sha256" or . == "size" or . == "source_commit" or . == "version")) and
    (.name | type == "string" and length > 0) and
    (.kind == "deb" or .kind == "executable") and
    (.os == "linux" or .os == "darwin") and
    (.architecture == "amd64" or .architecture == "arm64") and
    (.size | type == "number" and . > 0 and floor == .) and
    (.sha256 | test("^[0-9a-f]{64}$")) and
    (if .kind == "deb" then
      .os == "linux" and .package == "gizclaw" and .version == $version and
      .installed_path == "/usr/bin/gizclaw" and .source_commit == $source_commit
     else
      .os == "darwin" and
      ((has("package") or has("version") or has("installed_path") or has("source_commit")) | not)
     end))
  ' "$manifest" >/dev/null

expected_payloads="$(printf '%s\n' \
  gizclaw-darwin-amd64 gizclaw-darwin-arm64 \
  "gizclaw_${version}_amd64.deb" "gizclaw_${version}_arm64.deb" | LC_ALL=C sort)"
manifest_payloads="$(jq -r '.assets[].name' "$manifest")"
[[ "$manifest_payloads" == "$expected_payloads" ]] || { echo "manifest payload inventory mismatch" >&2; exit 1; }

while IFS= read -r name; do
  expected_size="$(jq -er --arg name "$name" '.assets[] | select(.name == $name) | .size' "$manifest")"
  expected_digest="$(jq -er --arg name "$name" '.assets[] | select(.name == $name) | .sha256' "$manifest")"
  [[ "$(wc -c <"$asset_dir/$name" | tr -d ' ')" == "$expected_size" ]] || { echo "size mismatch: $name" >&2; exit 1; }
  [[ "$(sha256sum "$asset_dir/$name" | awk '{print $1}')" == "$expected_digest" ]] || { echo "digest mismatch: $name" >&2; exit 1; }
done <<<"$expected_payloads"

checksums_expected="$(
  while IFS= read -r name; do
    printf '%s  %s\n' "$(sha256sum "$asset_dir/$name" | awk '{print $1}')" "$name"
  done < <(printf '%s\n%s\n' "$expected_payloads" release-manifest.json | LC_ALL=C sort)
)"
if ! cmp -s <(printf '%s\n' "$checksums_expected") "$asset_dir/SHA256SUMS"; then
  echo "SHA256SUMS mismatch" >&2
  exit 1
fi

for deb_arch in amd64 arm64; do
  deb="$asset_dir/gizclaw_${version}_${deb_arch}.deb"
  [[ "$(dpkg-deb --field "$deb" Package)" == gizclaw ]]
  [[ "$(dpkg-deb --field "$deb" Version)" == "$version" ]]
  [[ "$(dpkg-deb --field "$deb" Architecture)" == "$deb_arch" ]]
  [[ "$(dpkg-deb --field "$deb" X-GizClaw-Source-Commit)" == "$source_commit" ]]
done
if [[ "$requested_mode" == snapshot || "$requested_mode" == semver ]]; then
  for darwin_arch in amd64 arm64; do
    [[ -x "$asset_dir/gizclaw-darwin-$darwin_arch" ]] || { echo "local Darwin asset is not executable" >&2; exit 1; }
  done
fi

if [[ "$requested_mode" == draft || "$requested_mode" == published ]]; then
  expected_draft=false
  [[ "$requested_mode" != draft ]] || expected_draft=true
  jq -e \
    --arg tag "$tag" \
    --arg source_commit "$source_commit" \
    --argjson expected_draft "$expected_draft" '
      keys | all(. == "assets" or . == "draft" or . == "prerelease" or . == "tag_name" or . == "target_commitish")
    ' "$release_json" >/dev/null
  jq -e \
    --arg tag "$tag" \
    --arg source_commit "$source_commit" \
    --argjson expected_draft "$expected_draft" '
      .tag_name == $tag and
      .target_commitish == $source_commit and
      .draft == $expected_draft and
      .prerelease == false and
      (.assets | length == 6) and
      ([.assets[].name] | unique | length == 6) and
      all(.assets[];
        (keys | all(. == "name" or . == "size")) and
        (.name | type == "string" and length > 0) and
        (.size | type == "number" and . > 0 and floor == .))
    ' "$release_json" >/dev/null
  local_inventory="$(
    find "$asset_dir" -mindepth 1 -maxdepth 1 -type f -exec sh -c '
      for file do printf "%s\t%s\n" "$(basename "$file")" "$(wc -c <"$file" | tr -d " ")"; done
    ' sh {} + | LC_ALL=C sort
  )"
  remote_inventory="$(jq -r '.assets[] | [.name, (.size | tostring)] | @tsv' "$release_json" | LC_ALL=C sort)"
  [[ "$remote_inventory" == "$local_inventory" ]] || { echo "remote Release inventory or sizes mismatch" >&2; exit 1; }
fi

printf '%s\n' "validated ${requested_mode} release $tag"
