#!/usr/bin/env bash
# Offline regression tests for package and formal-release contracts.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for command_name in dpkg-deb jq sha256sum tar unzip zip; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

release_workflow="$repo_root/.github/workflows/release.yml"
ci_workflow="$repo_root/.github/workflows/ci.yml"
semver_publisher="$(awk '/^  publish-semver:/{selected=1} selected' "$release_workflow")"
grep -Fq "buildinfo.Version=\${BUILD_VERSION}" "$repo_root/build/Dockerfile"
grep -Fq "buildinfo.Commit=\${BUILD_COMMIT}" "$repo_root/build/Dockerfile"
[[ "$(grep -Fc "BUILD_VERSION: \${{ needs.prepare.outputs.version }}" "$release_workflow")" -eq 2 ]]
[[ "$(grep -Fc "BUILD_COMMIT: \${{ needs.prepare.outputs.source_commit }}" "$release_workflow")" -eq 1 ]]
grep -Fq "gizclaw version \$1" "$release_workflow"
grep -Fq 'tags:' "$release_workflow"
grep -Fq -- '- "v*"' "$release_workflow"
if grep -Eq '^    branches:|^  workflow_dispatch:|publish-snapshot|refs/tags/latest|gh release .*latest|0\.0\.0\+main' "$release_workflow"; then
  echo "release workflow must be triggered only by canonical SemVer tags" >&2
  exit 1
fi
if grep -Fq '".tmp/release/gizclaw-linux-' "$release_workflow"; then
  echo "release workflow must package Linux executables as Debian assets" >&2
  exit 1
fi
if grep -Eq 'gh release (delete|upload)|cleanup-tag|clobber|gh release .*latest|refs/tags/latest' <<<"$semver_publisher"; then
  echo "SemVer publisher contains a forbidden mutation or snapshot reference" >&2
  exit 1
fi
if grep -Eq 'gh api .*rulesets' <<<"$semver_publisher"; then
  echo "SemVer publisher must not query public rulesets with the permission-limited Actions token" >&2
  exit 1
fi
if grep -Fq 'immutable-releases' "$release_workflow"; then
  echo "release workflow must not require repository-wide immutable Release API access" >&2
  exit 1
fi
grep -Fq "\"\$GITHUB_API_URL/repos/\$GH_REPO/rulesets?includes_parents=true&per_page=100&page=\$rulesets_page\"" \
  <<<"$semver_publisher"
draft_transition="gh api --method PATCH \"repos/\$GH_REPO/releases/\$release_id\""
[[ "$(grep -Fc "$draft_transition" <<<"$semver_publisher")" -eq 1 ]] || {
  echo "SemVer publisher must contain exactly one draft-to-published transition" >&2
  exit 1
}
grep -Fq "build/find-release-by-tag.sh \"\$GH_REPO\" \"\$TAG\"" <<<"$semver_publisher"
grep -Fq "repos/\$GH_REPO/releases/assets/\$asset_id" <<<"$semver_publisher"
grep -Fq -- '- c-sdk' <<<"$semver_publisher"
grep -Fq -- '- terraform-provider' <<<"$semver_publisher"
grep -Fq -- '- flutter-sdk' <<<"$semver_publisher"
grep -Fq 'tools/flutter-sdk/package_archive.sh' "$release_workflow"
grep -Fq 'tools/flutter-sdk/consume_archives.sh' "$release_workflow"
grep -Fq 'build/build-terraform-provider.sh' "$release_workflow"
grep -Fq 'build/build-terraform-provider.sh' "$ci_workflow"
grep -Fq 'pattern: "*"' <<<"$semver_publisher"
grep -Fq 'merge-multiple: true' <<<"$semver_publisher"
if grep -Fq "releases/tags/\$TAG" <<<"$semver_publisher"; then
  echo "SemVer publisher must not use the published-only tag endpoint for draft lookup" >&2
  exit 1
fi
if grep -Eq 'gh release (create|delete|edit|upload)' "$ci_workflow"; then
  echo "pull-request CI must not publish or mutate a Release" >&2
  exit 1
fi

fixture_root="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-release-contract.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
tag=v0.0.0
version=0.0.0
source_commit=1111111111111111111111111111111111111111
snapshot_version=0.0.0+main.1.111111111111
fixture_binary="$(type -P true)"
[[ -n "$fixture_binary" ]] || { echo "could not locate the true executable" >&2; exit 2; }

expect_failure() {
  local label="$1"
  shift
  if "$@" >"$fixture_root/failure.stdout" 2>"$fixture_root/failure.stderr"; then
    echo "expected failure: $label" >&2
    exit 1
  fi
}

fake_bin="$fixture_root/bin"
mkdir -p "$fake_bin"
cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$*" == "api --paginate --slurp repos/GizClaw/gizclaw/releases?per_page=100" ]]
[[ "${MOCK_GH_FAILURE:-false}" != true ]] || exit 3
cat "$MOCK_RELEASE_PAGES"
EOF
chmod 0755 "$fake_bin/gh"

release_pages="$fixture_root/releases.json"
jq -n --arg tag "$tag" '[[
  {id:1,tag_name:"v9.9.9",draft:false},
  {id:2,tag_name:$tag,draft:true}
], [
  {id:3,tag_name:"v8.8.8",draft:false}
]]' >"$release_pages"
selected_release="$(
  PATH="$fake_bin:$PATH" MOCK_RELEASE_PAGES="$release_pages" \
    "$repo_root/build/find-release-by-tag.sh" GizClaw/gizclaw "$tag"
)"
jq -e --arg tag "$tag" '.id == 2 and .tag_name == $tag and .draft == true' \
  <<<"$selected_release" >/dev/null

jq -n '[[{id:1,tag_name:"v9.9.9",draft:false}]]' >"$release_pages"
set +e
PATH="$fake_bin:$PATH" MOCK_RELEASE_PAGES="$release_pages" \
  "$repo_root/build/find-release-by-tag.sh" GizClaw/gizclaw "$tag" >/dev/null
lookup_status=$?
set -e
[[ "$lookup_status" -eq 3 ]] || { echo "missing Release lookup must exit 3" >&2; exit 1; }

set +e
PATH="$fake_bin:$PATH" MOCK_RELEASE_PAGES="$release_pages" MOCK_GH_FAILURE=true \
  "$repo_root/build/find-release-by-tag.sh" GizClaw/gizclaw "$tag" >/dev/null 2>&1
lookup_status=$?
set -e
[[ "$lookup_status" -eq 1 ]] || { echo "GitHub API failure must fail closed" >&2; exit 1; }

jq -n --arg tag "$tag" '[[
  {id:1,tag_name:$tag,draft:true},
  {id:2,tag_name:$tag,draft:false}
]]' >"$release_pages"
expect_failure "duplicate exact-tag Releases" env \
  PATH="$fake_bin:$PATH" MOCK_RELEASE_PAGES="$release_pages" \
  "$repo_root/build/find-release-by-tag.sh" GizClaw/gizclaw "$tag"

make_fixture_deb() {
  local architecture="$1" output="$2" package_name="${3:-gizclaw}" package_version="${4:-$version}" package_source="${5:-$source_commit}"
  local root
  root="$fixture_root/deb-${architecture}-$(basename "$output")"
  rm -rf "$root"
  mkdir -p "$root/DEBIAN" "$root/usr/bin"
  install -m 0755 "$fixture_binary" "$root/usr/bin/gizclaw"
  cat >"$root/DEBIAN/control" <<EOF
Package: $package_name
Version: $package_version
Section: net
Priority: optional
Architecture: $architecture
Maintainer: GizClaw <opensource@gizclaw.com>
Depends: libc6
Description: GizClaw release contract fixture
X-GizClaw-Source-Commit: $package_source
EOF
  dpkg-deb --build --root-owner-group "$root" "$output" >/dev/null
}

provider_platforms=(darwin_amd64 darwin_arm64 linux_amd64 linux_arm64)

# Write the leading bytes of a Mach-O or ELF executable for one platform.
write_fixture_executable_header() {
  case "$1" in
    darwin_amd64) printf '\317\372\355\376\007\000\000\001' ;;
    darwin_arm64) printf '\317\372\355\376\014\000\000\001' ;;
    linux_amd64) printf '\177ELF\002\001\001\000\000\000\000\000\000\000\000\000\002\000\076\000' ;;
    linux_arm64) printf '\177ELF\002\001\001\000\000\000\000\000\000\000\000\000\002\000\267\000' ;;
    *) return 1 ;;
  esac
}

make_fixture_provider_zip() {
  local platform="$1" output="$2" binary_platform="${3:-$1}" entry="${4:-terraform-provider-gizclaw_v$version}" mode="${5:-0755}" root
  root="$fixture_root/provider-$(basename "$output")-$RANDOM"
  mkdir -p "$root"
  {
    write_fixture_executable_header "$binary_platform"
    printf '%s\n' "Terraform provider fixture for $platform"
  } >"$root/$entry"
  chmod "$mode" "$root/$entry"
  rm -f "$output"
  (cd "$root" && zip -q -X "$output" "$entry")
}

make_fixture_dart_package() {
  local package="$1" output="$2" package_version="${3:-$version}" root
  root="$fixture_root/dart-$(basename "$output")-$RANDOM"
  mkdir -p "$root/lib"
  printf 'name: %s\ndescription: fixture\nversion: %s\n' "$package" "$package_version" >"$root/pubspec.yaml"
  printf '%s\n' "// Flutter SDK fixture for $package" >"$root/lib/$package.dart"
  rm -f "$output"
  tar -C "$root" -czf "$output" pubspec.yaml lib
}

make_formal_payloads() {
  local directory="$1" c_sdk_archive platform
  mkdir -p "$directory"
  directory="$(cd "$directory" && pwd)"
  make_fixture_deb amd64 "$directory/gizclaw_${version}_amd64.deb"
  make_fixture_deb arm64 "$directory/gizclaw_${version}_arm64.deb"
  make_fixture_dart_package gizclaw "$directory/flutter-gizclaw-${version}.tar.gz"
  make_fixture_dart_package gizclaw_control "$directory/flutter-gizclaw_control-${version}.tar.gz"
  for platform in "${provider_platforms[@]}"; do
    make_fixture_provider_zip "$platform" "$directory/terraform-provider-gizclaw_${version}_${platform}.zip"
  done
  c_sdk_archive="gizclaw-c-sdk-${version}.tar.gz"
  printf '%s\n' "C SDK source archive fixture for $source_commit" >"$directory/$c_sdk_archive"
  printf '%s  %s\n' "$(sha256sum "$directory/$c_sdk_archive" | awk '{print $1}')" "$c_sdk_archive" \
    >"$directory/$c_sdk_archive.sha256"
}

payloads="$fixture_root/formal"
make_formal_payloads "$payloads"
expect_failure "latest manifest channel is unsupported" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$payloads" --tag latest --debian-version "$snapshot_version" --source-commit "$source_commit"
expect_failure "snapshot release mode is unsupported" "$repo_root/build/check-release.sh" \
  snapshot "$payloads" "$snapshot_version" "$source_commit"
"$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$payloads" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"
"$repo_root/build/check-release.sh" semver "$payloads" "$tag" "$source_commit"

release_assets='[]'
while IFS= read -r name; do
  size="$(wc -c <"$payloads/$name" | tr -d ' ')"
  release_assets="$(jq -c --arg name "$name" --argjson size "$size" '. + [{name:$name,size:$size}]' <<<"$release_assets")"
done < <(find "$payloads" -mindepth 1 -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort)
published_json="$fixture_root/published.json"
jq -n --arg tag "$tag" --arg source_commit "$source_commit" --argjson assets "$release_assets" \
  '{tag_name:$tag,target_commitish:$source_commit,draft:false,prerelease:false,assets:$assets}' >"$published_json"
"$repo_root/build/check-release.sh" published "$payloads" "$tag" "$source_commit" "$published_json"

draft_json="$fixture_root/draft.json"
jq '.draft = true' "$published_json" >"$draft_json"
"$repo_root/build/check-release.sh" draft "$payloads" "$tag" "$source_commit" "$draft_json"
expect_failure "draft is not a published idempotent match" "$repo_root/build/check-release.sh" \
  published "$payloads" "$tag" "$source_commit" "$draft_json"

partial_release_json="$fixture_root/partial-release.json"
jq '.assets = .assets[:-1]' "$published_json" >"$partial_release_json"
expect_failure "partial published Release" "$repo_root/build/check-release.sh" \
  published "$payloads" "$tag" "$source_commit" "$partial_release_json"

moved_release_json="$fixture_root/moved-release.json"
jq '.target_commitish = "2222222222222222222222222222222222222222"' "$published_json" >"$moved_release_json"
expect_failure "moved Release target" "$repo_root/build/check-release.sh" \
  published "$payloads" "$tag" "$source_commit" "$moved_release_json"

prerelease_json="$fixture_root/prerelease.json"
jq '.prerelease = true' "$published_json" >"$prerelease_json"
expect_failure "prerelease collision" "$repo_root/build/check-release.sh" \
  published "$payloads" "$tag" "$source_commit" "$prerelease_json"

payloads_second="$fixture_root/formal-second"
cp -a "$payloads" "$payloads_second"
rm "$payloads_second/release-manifest.json" "$payloads_second/SHA256SUMS"
"$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$payloads_second" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"
cmp "$payloads/release-manifest.json" "$payloads_second/release-manifest.json"
cmp "$payloads/SHA256SUMS" "$payloads_second/SHA256SUMS"

expect_failure "malformed tag prerelease" "$repo_root/build/check-release.sh" semver "$payloads" v0.0.0-rc.1 "$source_commit"
expect_failure "malformed tag leading zero" "$repo_root/build/check-release.sh" semver "$payloads" v00.0.0 "$source_commit"
expect_failure "short source identity" "$repo_root/build/check-release.sh" semver "$payloads" "$tag" 1111111

formal_extra="$fixture_root/formal-extra"
cp -a "$payloads" "$formal_extra"
touch "$formal_extra/unexpected"
expect_failure "formal extra asset" "$repo_root/build/check-release.sh" semver "$formal_extra" "$tag" "$source_commit"

formal_missing="$fixture_root/formal-missing"
cp -a "$payloads" "$formal_missing"
rm "$formal_missing/gizclaw_${version}_arm64.deb"
expect_failure "formal missing asset" "$repo_root/build/check-release.sh" semver "$formal_missing" "$tag" "$source_commit"

formal_digest="$fixture_root/formal-digest"
cp -a "$payloads" "$formal_digest"
printf '%s\n' tampered >>"$formal_digest/gizclaw_${version}_amd64.deb"
expect_failure "altered digest" "$repo_root/build/check-release.sh" semver "$formal_digest" "$tag" "$source_commit"

formal_checksums="$fixture_root/formal-checksums"
cp -a "$payloads" "$formal_checksums"
printf '\n' >>"$formal_checksums/SHA256SUMS"
expect_failure "non-canonical checksum file" "$repo_root/build/check-release.sh" semver "$formal_checksums" "$tag" "$source_commit"

formal_c_sdk_checksum="$fixture_root/formal-c-sdk-checksum"
cp -a "$payloads" "$formal_c_sdk_checksum"
printf '\n' >>"$formal_c_sdk_checksum/gizclaw-c-sdk-${version}.tar.gz.sha256"
expect_failure "non-canonical C SDK checksum sidecar" "$repo_root/build/check-release.sh" \
  semver "$formal_c_sdk_checksum" "$tag" "$source_commit"

formal_c_sdk_digest="$fixture_root/formal-c-sdk-digest"
cp -a "$payloads" "$formal_c_sdk_digest"
printf '%s\n' tampered >>"$formal_c_sdk_digest/gizclaw-c-sdk-${version}.tar.gz"
expect_failure "altered C SDK archive digest" "$repo_root/build/check-release.sh" \
  semver "$formal_c_sdk_digest" "$tag" "$source_commit"

formal_arch="$fixture_root/formal-architecture"
cp -a "$payloads" "$formal_arch"
jq --arg name "gizclaw_${version}_amd64.deb" '(.assets[] | select(.name == $name) | .architecture) = "arm64"' \
  "$formal_arch/release-manifest.json" >"$formal_arch/changed.json"
mv "$formal_arch/changed.json" "$formal_arch/release-manifest.json"
expect_failure "swapped Debian architecture" "$repo_root/build/check-release.sh" semver "$formal_arch" "$tag" "$source_commit"

formal_duplicate="$fixture_root/formal-duplicate"
cp -a "$payloads" "$formal_duplicate"
jq '(.assets[1].name) = .assets[0].name' "$formal_duplicate/release-manifest.json" >"$formal_duplicate/changed.json"
mv "$formal_duplicate/changed.json" "$formal_duplicate/release-manifest.json"
expect_failure "duplicate manifest asset" "$repo_root/build/check-release.sh" semver "$formal_duplicate" "$tag" "$source_commit"

formal_unstable="$fixture_root/formal-unstable"
cp -a "$payloads" "$formal_unstable"
jq '.run_id = 123' "$formal_unstable/release-manifest.json" >"$formal_unstable/changed.json"
mv "$formal_unstable/changed.json" "$formal_unstable/release-manifest.json"
expect_failure "per-run manifest value" "$repo_root/build/check-release.sh" semver "$formal_unstable" "$tag" "$source_commit"

jq -e --arg version "$version" --arg source_commit "$source_commit" '
  .schema_version == 5 and
  ([.assets[] | select(.kind == "terraform-provider")] | length == 4) and
  all(.assets[] | select(.kind == "terraform-provider");
    .provider == "gizclaw" and .version == $version and .source_commit == $source_commit and
    .executable == ("terraform-provider-gizclaw_v" + $version) and
    .name == ("terraform-provider-gizclaw_" + $version + "_" + .os + "_" + .architecture + ".zip"))
' "$payloads/release-manifest.json" >/dev/null
for platform in "${provider_platforms[@]}"; do
  grep -Eq "^[0-9a-f]{64}  terraform-provider-gizclaw_${version}_${platform}\.zip\$" "$payloads/SHA256SUMS"
done

formal_missing_provider="$fixture_root/formal-missing-provider"
cp -a "$payloads" "$formal_missing_provider"
rm "$formal_missing_provider/terraform-provider-gizclaw_${version}_darwin_arm64.zip"
expect_failure "formal missing provider archive" "$repo_root/build/check-release.sh" \
  semver "$formal_missing_provider" "$tag" "$source_commit"

formal_provider_digest="$fixture_root/formal-provider-digest"
cp -a "$payloads" "$formal_provider_digest"
make_fixture_provider_zip linux_amd64 "$formal_provider_digest/terraform-provider-gizclaw_${version}_linux_amd64.zip" linux_arm64
expect_failure "altered provider archive digest" "$repo_root/build/check-release.sh" \
  semver "$formal_provider_digest" "$tag" "$source_commit"

# Rebuild metadata after each provider mutation so only the archive check can fail.
rebuild_provider_case() {
  local label="$1" directory="$fixture_root/$2" platform="$3" binary_platform="$4" entry="$5" mode="$6"
  make_formal_payloads "$directory"
  make_fixture_provider_zip "$platform" "$directory/terraform-provider-gizclaw_${version}_${platform}.zip" \
    "$binary_platform" "$entry" "$mode"
  "$repo_root/build/build-release-manifest.sh" \
    --asset-dir "$directory" --tag "$tag" --debian-version "$version" --source-commit "$source_commit" >/dev/null
  expect_failure "$label" "$repo_root/build/check-release.sh" semver "$directory" "$tag" "$source_commit"
}
rebuild_provider_case "provider archive for the wrong architecture" provider-wrong-arch \
  linux_amd64 linux_arm64 "terraform-provider-gizclaw_v$version" 0755
rebuild_provider_case "provider archive for the wrong operating system" provider-wrong-os \
  darwin_arm64 linux_arm64 "terraform-provider-gizclaw_v$version" 0755
rebuild_provider_case "provider executable without execute mode" provider-mode \
  darwin_amd64 darwin_amd64 "terraform-provider-gizclaw_v$version" 0644

provider_wrong_entry="$fixture_root/provider-wrong-entry"
make_formal_payloads "$provider_wrong_entry"
make_fixture_provider_zip linux_arm64 "$provider_wrong_entry/terraform-provider-gizclaw_${version}_linux_arm64.zip" \
  linux_arm64 terraform-provider-gizclaw
expect_failure "provider archive with a non-Terraform executable name" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$provider_wrong_entry" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"

provider_extra_entry="$fixture_root/provider-extra-entry"
make_formal_payloads "$provider_extra_entry"
printf '%s\n' extra >"$fixture_root/README"
(cd "$fixture_root" && zip -q -X "$provider_extra_entry/terraform-provider-gizclaw_${version}_linux_amd64.zip" README)
expect_failure "provider archive with an extra entry" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$provider_extra_entry" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"

jq -e --arg version "$version" --arg source_commit "$source_commit" '
  [.assets[] | select(.kind == "dart-package") | {name,package,version,source_commit}] == [
    {name:("flutter-gizclaw-" + $version + ".tar.gz"),package:"gizclaw",version:$version,source_commit:$source_commit},
    {name:("flutter-gizclaw_control-" + $version + ".tar.gz"),package:"gizclaw_control",version:$version,source_commit:$source_commit}
  ]
' "$payloads/release-manifest.json" >/dev/null

formal_missing_dart="$fixture_root/formal-missing-dart-package"
cp -a "$payloads" "$formal_missing_dart"
rm "$formal_missing_dart/flutter-gizclaw_control-${version}.tar.gz"
expect_failure "formal missing Flutter SDK archive" "$repo_root/build/check-release.sh" \
  semver "$formal_missing_dart" "$tag" "$source_commit"

dart_wrong_version="$fixture_root/dart-wrong-version"
make_formal_payloads "$dart_wrong_version"
make_fixture_dart_package gizclaw "$dart_wrong_version/flutter-gizclaw-${version}.tar.gz" 0.0.1
expect_failure "Flutter SDK archive with another pubspec version" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$dart_wrong_version" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"

dart_swapped="$fixture_root/dart-swapped"
make_formal_payloads "$dart_swapped"
make_fixture_dart_package gizclaw "$dart_swapped/flutter-gizclaw_control-${version}.tar.gz"
expect_failure "Flutter SDK archive with another package name" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$dart_swapped" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"

dart_tampered="$fixture_root/dart-tampered"
make_formal_payloads "$dart_tampered"
"$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$dart_tampered" --tag "$tag" --debian-version "$version" --source-commit "$source_commit" >/dev/null
make_fixture_dart_package gizclaw "$dart_tampered/flutter-gizclaw-${version}.tar.gz" 0.0.1
expect_failure "Flutter SDK archive replaced after manifest" "$repo_root/build/check-release.sh" \
  semver "$dart_tampered" "$tag" "$source_commit"

wrong_metadata="$fixture_root/wrong-metadata"
make_formal_payloads "$wrong_metadata"
rm "$wrong_metadata/gizclaw_${version}_amd64.deb"
make_fixture_deb amd64 "$wrong_metadata/gizclaw_${version}_amd64.deb" wrong-package
expect_failure "invalid Debian package metadata" "$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$wrong_metadata" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"

wrong_deb_arch="$fixture_root/wrong-deb-architecture"
make_formal_payloads "$wrong_deb_arch"
rm "$wrong_deb_arch/gizclaw_${version}_amd64.deb"
make_fixture_deb arm64 "$wrong_deb_arch/gizclaw_${version}_amd64.deb"
"$repo_root/build/build-release-manifest.sh" \
  --asset-dir "$wrong_deb_arch" --tag "$tag" --debian-version "$version" --source-commit "$source_commit"
expect_failure "wrong Debian architecture" "$repo_root/build/check-release.sh" semver "$wrong_deb_arch" "$tag" "$source_commit"

if [[ "$(uname -s)" == Linux && "$(dpkg --print-architecture)" == amd64 ]]; then
  package_one="$fixture_root/package-one/gizclaw_${version}_amd64.deb"
  package_two="$fixture_root/package-two/gizclaw_${version}_amd64.deb"
  "$repo_root/build/package-deb.sh" \
    --binary "$fixture_binary" --version "$version" --source-commit "$source_commit" --source-epoch 1 \
    --architecture amd64 --output "$package_one"
  "$repo_root/build/package-deb.sh" \
    --binary "$fixture_binary" --version "$version" --source-commit "$source_commit" --source-epoch 1 \
    --architecture amd64 --output "$package_two"
  cmp "$package_one" "$package_two"
  "$repo_root/build/check-deb.sh" \
    --package "$package_one" --version "$version" --source-commit "$source_commit" --architecture amd64 --skip-runtime
  expect_failure "snapshot Debian version validation" "$repo_root/build/check-deb.sh" \
    --package "$package_one" --version "$snapshot_version" --source-commit "$source_commit" --architecture amd64 --skip-runtime
  expect_failure "snapshot Debian package construction" "$repo_root/build/package-deb.sh" \
    --binary "$fixture_binary" --version "$snapshot_version" --source-commit "$source_commit" --source-epoch 1 \
    --architecture amd64 --output "$fixture_root/snapshot-package.deb"
  wrong_dependencies_root="$fixture_root/wrong-dependencies-root"
  wrong_dependencies_package="$fixture_root/wrong-dependencies/gizclaw_${version}_amd64.deb"
  mkdir -p "$(dirname "$wrong_dependencies_package")"
  dpkg-deb --raw-extract "$package_one" "$wrong_dependencies_root"
  sed -i 's/^Depends: .*/Depends: libc6/' "$wrong_dependencies_root/DEBIAN/control"
  dpkg-deb --build --root-owner-group "$wrong_dependencies_root" "$wrong_dependencies_package" >/dev/null
  expect_failure "hand-maintained Debian dependencies" "$repo_root/build/check-deb.sh" \
    --package "$wrong_dependencies_package" --version "$version" --source-commit "$source_commit" --architecture amd64 --skip-runtime
  expect_failure "package output overwrite" "$repo_root/build/package-deb.sh" \
    --binary "$fixture_binary" --version "$version" --source-commit "$source_commit" --source-epoch 1 \
    --architecture amd64 --output "$package_one"
fi

printf '%s\n' "release contract tests passed"
