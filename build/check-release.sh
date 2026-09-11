#!/usr/bin/env bash
# Validate a protected formal-release inventory.

set -euo pipefail

requested_mode="${1:-}"
mode="$requested_mode"
asset_dir="${2:-}"
tag="${3:-}"
source_commit="${4:-}"
release_json="${5:-}"
case "$requested_mode" in
  semver)
    mode=release
    ;;
  draft | published)
    mode=release
    [[ -f "$release_json" && ! -L "$release_json" ]] || { echo "remote Release JSON must be a regular file" >&2; exit 2; }
    ;;
  *)
    echo "usage: $0 semver DIR TAG SOURCE_COMMIT | $0 draft|published DIR TAG SOURCE_COMMIT RELEASE_JSON" >&2
    exit 2
    ;;
esac
[[ "$mode" == release ]] || {
  echo "invalid release mode" >&2
  exit 2
}
[[ -d "$asset_dir" && ! -L "$asset_dir" ]] || { echo "asset directory must be regular" >&2; exit 2; }
for command_name in cmp dpkg-deb jq od sha256sum unzip; do
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

[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid canonical SemVer tag" >&2; exit 2; }
version="${tag#v}"
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
release_expected="$(printf '%s\n' \
  SHA256SUMS \
  "gizclaw-c-sdk-${version}.tar.gz" \
  "gizclaw-c-sdk-${version}.tar.gz.sha256" \
  "gizclaw_${version}_amd64.deb" \
  "gizclaw_${version}_arm64.deb" \
  release-manifest.json \
  "terraform-provider-gizclaw_${version}_darwin_amd64.zip" \
  "terraform-provider-gizclaw_${version}_darwin_arm64.zip" \
  "terraform-provider-gizclaw_${version}_linux_amd64.zip" \
  "terraform-provider-gizclaw_${version}_linux_arm64.zip" | LC_ALL=C sort)"
assert_inventory "$release_expected"

manifest="$asset_dir/release-manifest.json"
jq -e \
  --arg tag "$tag" \
  --arg version "$version" --arg source_commit "$source_commit" '
  keys == ["assets","debian_version","go_module","go_module_version","release_channel","repository","schema_version","source_commit","tag","workflow"] and
  .schema_version == 4 and
  .repository == "GizClaw/gizclaw" and
  .go_module == "github.com/GizClaw/gizclaw-go" and
  .release_channel == "stable" and
  .tag == $tag and
  .go_module_version == $tag and
  .debian_version == $version and
  .source_commit == $source_commit and .workflow == ".github/workflows/release.yml" and
  (.assets | length == 7) and
  ([.assets[].name] == ([.assets[].name] | sort)) and
  ([.assets[].name] | unique | length == 7) and
  ([.assets[] | {name,kind,os,architecture}] == [
    {name:("gizclaw-c-sdk-" + $version + ".tar.gz"),kind:"source",os:null,architecture:null},
    {name:("gizclaw_" + $version + "_amd64.deb"),kind:"deb",os:"linux",architecture:"amd64"},
    {name:("gizclaw_" + $version + "_arm64.deb"),kind:"deb",os:"linux",architecture:"arm64"},
    {name:("terraform-provider-gizclaw_" + $version + "_darwin_amd64.zip"),kind:"terraform-provider",os:"darwin",architecture:"amd64"},
    {name:("terraform-provider-gizclaw_" + $version + "_darwin_arm64.zip"),kind:"terraform-provider",os:"darwin",architecture:"arm64"},
    {name:("terraform-provider-gizclaw_" + $version + "_linux_amd64.zip"),kind:"terraform-provider",os:"linux",architecture:"amd64"},
    {name:("terraform-provider-gizclaw_" + $version + "_linux_arm64.zip"),kind:"terraform-provider",os:"linux",architecture:"arm64"}
  ]) and
  all(.assets[];
    (keys | all(. == "architecture" or . == "executable" or . == "installed_path" or . == "kind" or . == "module" or . == "name" or . == "os" or . == "package" or . == "provider" or . == "sha256" or . == "size" or . == "source_commit" or . == "version")) and
    (.name | type == "string" and length > 0) and
    (.kind == "deb" or .kind == "source" or .kind == "terraform-provider") and
    (.size | type == "number" and . > 0 and floor == .) and
    (.sha256 | test("^[0-9a-f]{64}$")) and
    (if .kind == "deb" then
      .os == "linux" and .package == "gizclaw" and .version == $version and
      .installed_path == "/usr/bin/gizclaw" and .source_commit == $source_commit and
      ((has("module") or has("provider") or has("executable")) | not)
     elif .kind == "terraform-provider" then
      .provider == "gizclaw" and .version == $version and
      .executable == ("terraform-provider-gizclaw_v" + $version) and .source_commit == $source_commit and
      ((has("module") or has("package") or has("installed_path")) | not)
     else
      .name == ("gizclaw-c-sdk-" + $version + ".tar.gz") and
      .module == "gizclaw_c_sdk" and .version == $version and .source_commit == $source_commit and
      ((has("os") or has("architecture") or has("package") or has("installed_path") or has("provider") or has("executable")) | not)
     end))
  ' "$manifest" >/dev/null

provider_platforms=(darwin_amd64 darwin_arm64 linux_amd64 linux_arm64)
expected_payloads="$(
  {
    printf '%s\n' \
      "gizclaw-c-sdk-${version}.tar.gz" \
      "gizclaw_${version}_amd64.deb" "gizclaw_${version}_arm64.deb"
    for platform in "${provider_platforms[@]}"; do
      printf '%s\n' "terraform-provider-gizclaw_${version}_${platform}.zip"
    done
  } | LC_ALL=C sort
)"
manifest_payloads="$(jq -r '.assets[].name' "$manifest")"
[[ "$manifest_payloads" == "$expected_payloads" ]] || { echo "manifest payload inventory mismatch" >&2; exit 1; }

while IFS= read -r name; do
  expected_size="$(jq -er --arg name "$name" '.assets[] | select(.name == $name) | .size' "$manifest")"
  expected_digest="$(jq -er --arg name "$name" '.assets[] | select(.name == $name) | .sha256' "$manifest")"
  [[ "$(wc -c <"$asset_dir/$name" | tr -d ' ')" == "$expected_size" ]] || { echo "size mismatch: $name" >&2; exit 1; }
  [[ "$(sha256sum "$asset_dir/$name" | awk '{print $1}')" == "$expected_digest" ]] || { echo "digest mismatch: $name" >&2; exit 1; }
done <<<"$expected_payloads"

c_sdk_archive="gizclaw-c-sdk-${version}.tar.gz"
c_sdk_checksum="${c_sdk_archive}.sha256"
expected_c_sdk_checksum="$(sha256sum "$asset_dir/$c_sdk_archive" | awk '{print $1}')  $c_sdk_archive"
cmp -s <(printf '%s\n' "$expected_c_sdk_checksum") "$asset_dir/$c_sdk_checksum" || {
  echo "C SDK archive checksum sidecar mismatch" >&2
  exit 1
}

checksums_expected="$(
  while IFS= read -r name; do
    printf '%s  %s\n' "$(sha256sum "$asset_dir/$name" | awk '{print $1}')" "$name"
  done < <(printf '%s\n%s\n%s\n' "$expected_payloads" "$c_sdk_checksum" release-manifest.json | LC_ALL=C sort)
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

# Each provider archive holds exactly one executable in Terraform's
# terraform-provider-<type>_v<version> form, built for the named platform.
provider_executable="terraform-provider-gizclaw_v${version}"
provider_header_matches() {
  local header
  header="$(od -An -tx1 -N20 "$1" | tr -d ' \n')"
  case "$2" in
    linux_amd64) [[ "${header:0:12}" == 7f454c460201 && "${header:36:4}" == 3e00 ]] ;;
    linux_arm64) [[ "${header:0:12}" == 7f454c460201 && "${header:36:4}" == b700 ]] ;;
    darwin_amd64) [[ "${header:0:16}" == cffaedfe07000001 ]] ;;
    darwin_arm64) [[ "${header:0:16}" == cffaedfe0c000001 ]] ;;
    *) return 1 ;;
  esac
}
provider_extract_dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-provider-check.XXXXXX")"
trap 'rm -rf "$provider_extract_dir"' EXIT
for platform in "${provider_platforms[@]}"; do
  archive="$asset_dir/terraform-provider-gizclaw_${version}_${platform}.zip"
  [[ "$(unzip -Z1 "$archive")" == "$provider_executable" ]] || {
    echo "Terraform provider archive must contain only $provider_executable: $(basename "$archive")" >&2
    exit 1
  }
  [[ "$(unzip -Z "$archive" "$provider_executable" | awk 'NR == 1 { print $1 }')" == -rwxr-xr-x ]] || {
    echo "Terraform provider executable must have mode 0755: $(basename "$archive")" >&2
    exit 1
  }
  unzip -p "$archive" "$provider_executable" >"$provider_extract_dir/$platform"
  provider_header_matches "$provider_extract_dir/$platform" "$platform" || {
    echo "Terraform provider executable does not match $platform: $(basename "$archive")" >&2
    exit 1
  }
  rm -f "$provider_extract_dir/$platform"
done

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
      (.assets | length == 10) and
      ([.assets[].name] | unique | length == 10) and
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
