#!/usr/bin/env bash
# Offline regression tests for package and formal-release contracts.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
for command_name in dpkg-deb jq sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

release_workflow="$repo_root/.github/workflows/release.yml"
ci_workflow="$repo_root/.github/workflows/ci.yml"
semver_publisher="$(awk '/^  publish-semver:/{selected=1} selected' "$release_workflow")"
grep -Fq "buildinfo.Version=\${BUILD_VERSION}" "$repo_root/build/Dockerfile"
grep -Fq "buildinfo.Commit=\${BUILD_COMMIT}" "$repo_root/build/Dockerfile"
[[ "$(grep -Fc "BUILD_VERSION: \${{ needs.prepare.outputs.version }}" "$release_workflow")" -eq 4 ]]
[[ "$(grep -Fc "BUILD_COMMIT: \${{ needs.prepare.outputs.source_commit }}" "$release_workflow")" -eq 2 ]]
grep -Fq "gizclaw version \$BUILD_VERSION" "$release_workflow"
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
draft_transition="gh release edit \"\$TAG\" --draft=false"
[[ "$(grep -Fc "$draft_transition" <<<"$semver_publisher")" -eq 1 ]] || {
  echo "SemVer publisher must contain exactly one draft-to-published transition" >&2
  exit 1
}
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

make_formal_payloads() {
  local directory="$1"
  mkdir -p "$directory"
  make_fixture_deb amd64 "$directory/gizclaw_${version}_amd64.deb"
  make_fixture_deb arm64 "$directory/gizclaw_${version}_arm64.deb"
  install -m 0755 "$fixture_binary" "$directory/gizclaw-darwin-amd64"
  install -m 0755 "$fixture_binary" "$directory/gizclaw-darwin-arm64"
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

downloaded_payloads="$fixture_root/downloaded-formal"
cp -a "$payloads" "$downloaded_payloads"
chmod 0644 "$downloaded_payloads"/gizclaw-darwin-*
"$repo_root/build/check-release.sh" published "$downloaded_payloads" "$tag" "$source_commit" "$published_json"
expect_failure "local Darwin payload without executable mode" "$repo_root/build/check-release.sh" \
  semver "$downloaded_payloads" "$tag" "$source_commit"

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
rm "$formal_missing/gizclaw-darwin-arm64"
expect_failure "formal missing asset" "$repo_root/build/check-release.sh" semver "$formal_missing" "$tag" "$source_commit"

formal_digest="$fixture_root/formal-digest"
cp -a "$payloads" "$formal_digest"
printf '%s\n' tampered >>"$formal_digest/gizclaw-darwin-amd64"
expect_failure "altered digest" "$repo_root/build/check-release.sh" semver "$formal_digest" "$tag" "$source_commit"

formal_checksums="$fixture_root/formal-checksums"
cp -a "$payloads" "$formal_checksums"
printf '\n' >>"$formal_checksums/SHA256SUMS"
expect_failure "non-canonical checksum file" "$repo_root/build/check-release.sh" semver "$formal_checksums" "$tag" "$source_commit"

formal_arch="$fixture_root/formal-architecture"
cp -a "$payloads" "$formal_arch"
jq '(.assets[] | select(.name == "gizclaw-darwin-amd64") | .architecture) = "arm64"' \
  "$formal_arch/release-manifest.json" >"$formal_arch/changed.json"
mv "$formal_arch/changed.json" "$formal_arch/release-manifest.json"
expect_failure "swapped macOS architecture" "$repo_root/build/check-release.sh" semver "$formal_arch" "$tag" "$source_commit"

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
