#!/usr/bin/env bash
# Assemble one reproducible GizClaw Debian package from a prebuilt Linux binary.

set -euo pipefail

binary=
tag=
source_commit=
source_epoch=
architecture=
output=

while (($# > 0)); do
  case "$1" in
    --binary) binary="${2:-}"; shift 2 ;;
    --tag) tag="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    --source-epoch) source_epoch="${2:-}"; shift 2 ;;
    --architecture) architecture="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

usage() {
  echo "usage: $0 --binary PATH --tag vMAJOR.MINOR.PATCH --source-commit SHA --source-epoch UNIX --architecture amd64|arm64 --output PATH" >&2
  exit 2
}

[[ -n "$binary" && -n "$tag" && -n "$source_commit" && -n "$source_epoch" && -n "$architecture" && -n "$output" ]] || usage
[[ "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo "tag must be a canonical stable vMAJOR.MINOR.PATCH" >&2
  exit 2
}
version="${tag#v}"
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "source commit must be full lowercase hex" >&2; exit 2; }
[[ "$source_epoch" =~ ^[0-9]+$ ]] || { echo "source epoch must be a non-negative integer" >&2; exit 2; }
case "$architecture" in amd64 | arm64) ;; *) echo "unsupported architecture: $architecture" >&2; exit 2 ;; esac

for command_name in dpkg-deb dpkg-shlibdeps file readelf; do
  command -v "$command_name" >/dev/null 2>&1 || {
    echo "required command not found: $command_name" >&2
    exit 2
  }
done
[[ -f "$binary" && ! -L "$binary" && -s "$binary" && -x "$binary" ]] || {
  echo "binary must be a non-empty regular executable, not a symlink" >&2
  exit 2
}
[[ ! -e "$output" && ! -L "$output" ]] || { echo "refusing to overwrite output: $output" >&2; exit 1; }

machine="$(readelf -h "$binary" | awk -F: '/^[[:space:]]*Machine:/ { sub(/^[[:space:]]+/, "", $2); print $2 }')"
case "$architecture:$machine" in
  amd64:"Advanced Micro Devices X86-64" | arm64:AArch64) ;;
  *) echo "binary machine '$machine' does not match Debian architecture '$architecture'" >&2; exit 1 ;;
esac

work_dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-package.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
package_root="$work_dir/root"
mkdir -p "$package_root/DEBIAN" "$package_root/usr/bin" "$work_dir/debian" "$(dirname "$output")"
install -m 0755 "$binary" "$package_root/usr/bin/gizclaw"

cat >"$work_dir/debian/control" <<'EOF'
Source: gizclaw
Section: net
Priority: optional
Maintainer: GizClaw <opensource@gizclaw.com>

Package: gizclaw
Architecture: any
Depends: ${shlibs:Depends}
Description: GizClaw server and edge runtime
EOF
dependency_output="$(cd "$work_dir" && dpkg-shlibdeps -O -e"$package_root/usr/bin/gizclaw" 2>"$work_dir/dpkg-shlibdeps.stderr")" || {
  cat "$work_dir/dpkg-shlibdeps.stderr" >&2
  exit 1
}
dependencies="${dependency_output#shlibs:Depends=}"
[[ "$dependency_output" == shlibs:Depends=* && -n "$dependencies" ]] || {
  echo "dpkg-shlibdeps did not derive any shared-library dependencies" >&2
  exit 1
}
if grep -Eqi 'not found|unresolved' "$work_dir/dpkg-shlibdeps.stderr"; then
  cat "$work_dir/dpkg-shlibdeps.stderr" >&2
  echo "dpkg-shlibdeps reported unresolved or unsafe dependency output" >&2
  exit 1
fi

cat >"$package_root/DEBIAN/control" <<EOF
Package: gizclaw
Version: $version
Section: net
Priority: optional
Architecture: $architecture
Maintainer: GizClaw <opensource@gizclaw.com>
Depends: $dependencies
Description: GizClaw server and edge runtime
 GizClaw command-line runtime built from a protected source tag.
X-GizClaw-Source-Commit: $source_commit
EOF

find "$package_root" -print0 | LC_ALL=C sort -z | xargs -0 touch --no-dereference --date="@$source_epoch"
temporary_output="$work_dir/$(basename "$output")"
SOURCE_DATE_EPOCH="$source_epoch" TZ=UTC dpkg-deb \
  --build \
  --root-owner-group \
  --uniform-compression \
  -Zxz \
  -z9 \
  "$package_root" \
  "$temporary_output" >/dev/null
install -m 0644 "$temporary_output" "$output"

printf '%s\n' "built $(basename "$output")"
