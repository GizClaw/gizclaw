#!/usr/bin/env bash
# Validate GizClaw package metadata, payload, dependencies, and native lifecycle.

set -euo pipefail

package_path=
version=
source_commit=
architecture=
skip_runtime=false

while (($# > 0)); do
  case "$1" in
    --package) package_path="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    --architecture) architecture="${2:-}"; shift 2 ;;
    --skip-runtime) skip_runtime=true; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -n "$package_path" && -n "$version" && -n "$source_commit" && -n "$architecture" ]] || {
  echo "usage: $0 --package PATH --version DEBIAN_VERSION --source-commit SHA --architecture amd64|arm64 [--skip-runtime]" >&2
  exit 2
}
if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "invalid stable Debian version" >&2
  exit 2
fi
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
case "$architecture" in amd64 | arm64) ;; *) echo "unsupported architecture: $architecture" >&2; exit 2 ;; esac
[[ -f "$package_path" && ! -L "$package_path" && -s "$package_path" ]] || { echo "package is not a regular non-empty file" >&2; exit 1; }

for command_name in dpkg-deb dpkg-shlibdeps tar; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

expected_name="gizclaw_${version}_${architecture}.deb"
[[ "$(basename "$package_path")" == "$expected_name" ]] || { echo "unexpected package filename" >&2; exit 1; }
[[ "$(dpkg-deb --field "$package_path" Package)" == gizclaw ]]
[[ "$(dpkg-deb --field "$package_path" Version)" == "$version" ]]
[[ "$(dpkg-deb --field "$package_path" Architecture)" == "$architecture" ]]
[[ "$(dpkg-deb --field "$package_path" X-GizClaw-Source-Commit)" == "$source_commit" ]]
dependencies="$(dpkg-deb --field "$package_path" Depends)"
[[ -n "$dependencies" ]] || { echo "package Depends must be derived and non-empty" >&2; exit 1; }

work_dir="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-check-deb.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT
dpkg-deb --control "$package_path" "$work_dir/control"
for forbidden_control in preinst postinst prerm postrm conffiles triggers; do
  [[ ! -e "$work_dir/control/$forbidden_control" ]] || { echo "package must not contain $forbidden_control" >&2; exit 1; }
done

dpkg-deb --fsys-tarfile "$package_path" >"$work_dir/data.tar"
mapfile -t payload_files < <(tar -tf "$work_dir/data.tar" | sed 's#^\./##' | sed -e '/^$/d' -e '/\/$/d')
[[ "${#payload_files[@]}" -eq 1 && "${payload_files[0]}" == usr/bin/gizclaw ]] || {
  echo "package must install exactly /usr/bin/gizclaw" >&2
  printf 'payload: %s\n' "${payload_files[@]}" >&2
  exit 1
}
payload_listing="$(tar --numeric-owner -tvf "$work_dir/data.tar" | awk '$NF == "./usr/bin/gizclaw" { print }')"
[[ "$payload_listing" =~ ^-rwxr-xr-x[[:space:]]+0/0[[:space:]] ]] || {
  echo "package executable must be root/root mode 0755" >&2
  exit 1
}

mkdir -p "$work_dir/extracted" "$work_dir/debian"
tar -xf "$work_dir/data.tar" -C "$work_dir/extracted" ./usr/bin/gizclaw
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
derived_output="$(cd "$work_dir" && dpkg-shlibdeps -O -e"$work_dir/extracted/usr/bin/gizclaw" 2>"$work_dir/dpkg-shlibdeps.stderr")" || {
  cat "$work_dir/dpkg-shlibdeps.stderr" >&2
  exit 1
}
derived_dependencies="${derived_output#shlibs:Depends=}"
[[ "$derived_output" == shlibs:Depends=* && -n "$derived_dependencies" && "$dependencies" == "$derived_dependencies" ]] || {
  echo "package Depends does not equal the dependencies derived from its ELF payload" >&2
  exit 1
}
if grep -Eqi 'not found|unresolved' "$work_dir/dpkg-shlibdeps.stderr"; then
  cat "$work_dir/dpkg-shlibdeps.stderr" >&2
  echo "packaged ELF contains unresolved shared-library dependencies" >&2
  exit 1
fi

if [[ "$skip_runtime" == true ]]; then
  printf '%s\n' "validated metadata for $expected_name"
  exit 0
fi
command -v docker >/dev/null 2>&1 || { echo "required command not found: docker" >&2; exit 2; }
package_dir="$(cd "$(dirname "$package_path")" && pwd)"
docker run --rm --platform "linux/$architecture" \
  --volume "$package_dir:/packages:ro" \
  ubuntu:24.04 \
  bash -euc '
    export DEBIAN_FRONTEND=noninteractive
    package="/packages/$1"
    expected_version="$2"
    version_format="\${Version}"
    apt-get update >/dev/null
    apt-get install --no-install-recommends -y "$package" >/dev/null
    test "$(dpkg-query -W -f="$version_format" gizclaw)" = "$expected_version"
    test "$(stat -c %U:%G /usr/bin/gizclaw)" = root:root
    test "$(stat -c %a /usr/bin/gizclaw)" = 755
    test "$(/usr/bin/gizclaw --version)" = "gizclaw version $expected_version"
    /usr/bin/gizclaw --help >/dev/null
    apt-get remove -y gizclaw >/dev/null
    test ! -e /usr/bin/gizclaw
    apt-get install --no-install-recommends -y "$package" >/dev/null
    printf "%s\n" corrupted > /usr/bin/gizclaw
    chmod 0755 /usr/bin/gizclaw
    apt-get install --reinstall --no-install-recommends -y "$package" >/dev/null
    test "$(/usr/bin/gizclaw --version)" = "gizclaw version $expected_version"
    /usr/bin/gizclaw --help >/dev/null
  ' bash "$expected_name" "$version"

printf '%s\n' "validated $expected_name"
