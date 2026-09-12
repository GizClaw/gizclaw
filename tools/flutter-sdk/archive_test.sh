#!/usr/bin/env bash
# Offline regression tests for the pub-hosted Flutter SDK archive contract.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for command_name in cmp git python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

fixture_root="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-flutter-sdk-contract.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
version=0.0.0
source_commit="$(git -C "$repo_root" rev-parse HEAD)"
source_epoch="$(git -C "$repo_root" show -s --format=%ct HEAD)"

expect_failure() {
  local label="$1"
  shift
  if "$@" >"$fixture_root/failure.stdout" 2>"$fixture_root/failure.stderr"; then
    echo "expected failure: $label" >&2
    exit 1
  fi
}

package_archive() {
  "$repo_root/tools/flutter-sdk/package_archive.sh" \
    --package "$1" --version "$version" --source-commit "$source_commit" --source-epoch "$source_epoch" --output "$2"
}

for package in gizclaw gizclaw_control; do
  archive_one="$fixture_root/flutter-$package-$version-a.tar.gz"
  archive_two="$fixture_root/flutter-$package-$version-b.tar.gz"
  package_archive "$package" "$archive_one"
  package_archive "$package" "$archive_two"
  cmp "$archive_one" "$archive_two"
  "$repo_root/tools/flutter-sdk/verify_archive.sh" --archive "$archive_one" --package "$package" --version "$version"
  expect_failure "$package archive for another package" "$repo_root/tools/flutter-sdk/verify_archive.sh" \
    --archive "$archive_one" --package "$([[ "$package" == gizclaw ]] && echo gizclaw_control || echo gizclaw)" \
    --version "$version"
  expect_failure "$package archive for another version" "$repo_root/tools/flutter-sdk/verify_archive.sh" \
    --archive "$archive_one" --package "$package" --version 0.0.1
done

archive="$fixture_root/flutter-gizclaw_control-$version-a.tar.gz"
expect_failure "unknown package" "$repo_root/tools/flutter-sdk/package_archive.sh" \
  --package giztest --version "$version" --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "invalid version" "$repo_root/tools/flutter-sdk/package_archive.sh" \
  --package gizclaw --version 00.0.0 --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "short source commit" "$repo_root/tools/flutter-sdk/package_archive.sh" \
  --package gizclaw --version "$version" --source-commit 1111111 --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "mismatched source epoch" "$repo_root/tools/flutter-sdk/package_archive.sh" \
  --package gizclaw --version "$version" --source-commit "$source_commit" --source-epoch 1 \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "archive overwrite" package_archive gizclaw_control "$archive"

rewrite_archive() {
  local mode="$1" destination="$2"
  python3 - "$archive" "$destination" "$mode" <<'PY'
import gzip
import io
import tarfile
import sys

source_path, destination_path, mode = sys.argv[1:]
with tarfile.open(source_path, "r:gz") as source:
    members = [(member, source.extractfile(member).read() if member.isreg() else None) for member in source.getmembers()]
archive_mtime = members[0][0].mtime


def file_member(name, payload, **changes):
    info = tarfile.TarInfo(name)
    info.mode = 0o644
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.mtime = archive_mtime
    info.size = len(payload) if payload is not None else 0
    for key, value in changes.items():
        setattr(info, key, value)
    return info, payload


if mode == "extra":
    members.append(file_member("test/unexpected_test.dart", b"x"))
elif mode == "root-directory":
    for member, _ in members:
        member.name = "gizclaw_control/" + member.name
elif mode == "symlink":
    members.append(file_member("lib/escape.dart", None, type=tarfile.SYMTYPE, linkname="../../escape"))
elif mode == "traversal":
    members.append(file_member("lib/../../escape.dart", b"x"))
elif mode == "duplicate":
    members.append(next(item for item in members if item[0].name == "pubspec.yaml"))
elif mode == "mode":
    next(member for member, _ in members if member.name == "pubspec.yaml").mode = 0o755
elif mode == "timestamp":
    next(member for member, _ in members if member.name == "LICENSE").mtime = archive_mtime + 1
elif mode == "missing":
    members = [item for item in members if item[0].name != "lib/gizclaw_control.dart"]
elif mode in {"version", "path-dependency"}:
    changed = []
    for member, payload in members:
        if member.name == "pubspec.yaml":
            text = payload.decode()
            if mode == "version":
                text = text.replace("version: 0.0.0", "version: 0.0.1")
            else:
                text += "dependency_overrides:\n  http:\n    path: ../http\n"
            payload = text.encode()
            member.size = len(payload)
        changed.append((member, payload))
    members = changed
else:
    raise SystemExit(f"unknown rewrite mode: {mode}")

with open(destination_path, "xb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=archive_mtime) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as target:
            for member, payload in members:
                target.addfile(member, io.BytesIO(payload) if payload is not None else None)
PY
}

for mode in extra root-directory symlink traversal duplicate mode timestamp missing version path-dependency; do
  rewritten="$fixture_root/$mode.tar.gz"
  rewrite_archive "$mode" "$rewritten"
  expect_failure "$mode archive" "$repo_root/tools/flutter-sdk/verify_archive.sh" \
    --archive "$rewritten" --package gizclaw_control --version "$version"
done

printf '%s\n' "Flutter SDK archive contract tests passed"
