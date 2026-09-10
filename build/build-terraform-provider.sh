#!/usr/bin/env bash
# Cross-build the cgo-free Terraform provider and package it for Terraform.

set -euo pipefail

version=
source_commit=
source_epoch=
output_dir=
while (($# > 0)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    --source-epoch) source_epoch="${2:-}"; shift 2 ;;
    --output-dir) output_dir="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -n "$version" && -n "$source_commit" && -n "$source_epoch" && -n "$output_dir" ]] || {
  echo "usage: $0 --version MAJOR.MINOR.PATCH --source-commit SHA --source-epoch SECONDS --output-dir DIR" >&2
  exit 2
}
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid canonical stable version" >&2; exit 2; }
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
# Zip timestamps cannot represent times before 1980-01-01.
[[ "$source_epoch" =~ ^[1-9][0-9]*$ && "$source_epoch" -ge 315532800 ]] || { echo "invalid source epoch" >&2; exit 2; }
for command_name in go zip unzip; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
executable="terraform-provider-gizclaw_v${version}"
stamp="$(date -u -d "@$source_epoch" +%Y%m%d%H%M.%S 2>/dev/null || date -u -r "$source_epoch" +%Y%m%d%H%M.%S)"
platforms=(darwin/amd64 darwin/arm64 linux/amd64 linux/arm64)

mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
for platform in "${platforms[@]}"; do
  archive="$output_dir/terraform-provider-gizclaw_${version}_${platform%/*}_${platform#*/}.zip"
  [[ ! -e "$archive" && ! -L "$archive" ]] || { echo "refusing to overwrite $archive" >&2; exit 1; }
done

work="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-terraform-provider.XXXXXX")"
trap 'rm -rf "$work"' EXIT

sha256_line() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    shasum -a 256 "$1"
  fi
}

for platform in "${platforms[@]}"; do
  goos="${platform%/*}"
  goarch="${platform#*/}"
  platform_dir="$work/${goos}_${goarch}"
  mkdir -p "$platform_dir"
  binary="$platform_dir/$executable"
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOFLAGS='' go build \
      -trimpath \
      -buildvcs=false \
      -ldflags "-s -w -buildid= -X main.version=${version}" \
      -o "$binary" \
      ./cmd/terraform-provider-gizclaw
  )

  build_info="$(go version -m "$binary")"
  for setting in "CGO_ENABLED=0" "GOOS=$goos" "GOARCH=$goarch"; do
    grep -Eq "^[[:space:]]+build[[:space:]]+${setting}\$" <<<"$build_info" || {
      echo "$binary was not built with $setting" >&2
      exit 1
    }
  done

  chmod 0755 "$binary"
  TZ=UTC touch -t "$stamp" "$binary"
  archive="$output_dir/terraform-provider-gizclaw_${version}_${goos}_${goarch}.zip"
  (cd "$platform_dir" && TZ=UTC zip -q -X -D "$archive" "$executable")
  [[ "$(unzip -Z1 "$archive")" == "$executable" ]] || { echo "unexpected archive entries: $archive" >&2; exit 1; }
  sha256_line "$archive"
done
