#!/usr/bin/env bash
# Offline regression tests for the standalone C SDK source archive contract.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for command_name in cmp git python3 sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

fixture_root="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-c-sdk-contract.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
version=0.0.0
source_commit="$(git -C "$repo_root" rev-parse HEAD)"
source_epoch="$(git -C "$repo_root" show -s --format=%ct HEAD)"
archive_one="$fixture_root/gizclaw-c-sdk-$version-a.tar.gz"
archive_two="$fixture_root/gizclaw-c-sdk-$version-b.tar.gz"

expect_failure() {
  local label="$1"
  shift
  if "$@" >"$fixture_root/failure.stdout" 2>"$fixture_root/failure.stderr"; then
    echo "expected failure: $label" >&2
    exit 1
  fi
}

"$repo_root/tools/c-sdk/package_source.sh" \
  --version "$version" \
  --source-commit "$source_commit" \
  --source-epoch "$source_epoch" \
  --output "$archive_one"
"$repo_root/tools/c-sdk/package_source.sh" \
  --version "$version" \
  --source-commit "$source_commit" \
  --source-epoch "$source_epoch" \
  --output "$archive_two"
cmp "$archive_one" "$archive_two"
"$repo_root/tools/c-sdk/verify_source_archive.sh" \
  --archive "$archive_one" \
  --version "$version" \
  --source-commit "$source_commit"

expect_failure "invalid version" "$repo_root/tools/c-sdk/package_source.sh" \
  --version 00.0.0 --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "short source commit" "$repo_root/tools/c-sdk/package_source.sh" \
  --version "$version" --source-commit 1111111 --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "mismatched source epoch" "$repo_root/tools/c-sdk/package_source.sh" \
  --version "$version" --source-commit "$source_commit" --source-epoch 1 \
  --output "$fixture_root/invalid.tar.gz"
expect_failure "archive overwrite" "$repo_root/tools/c-sdk/package_source.sh" \
  --version "$version" --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$archive_one"

tampered_bytes="$fixture_root/tampered-bytes.tar.gz"
cp "$archive_one" "$tampered_bytes"
printf '%s  %s\n' "$(sha256sum "$tampered_bytes" | awk '{print $1}')" "$(basename "$tampered_bytes")" \
  >"$tampered_bytes.sha256"
printf 'tampered\n' >>"$tampered_bytes"
expect_failure "stale checksum" "$repo_root/tools/c-sdk/verify_source_archive.sh" \
  --archive "$tampered_bytes" --version "$version" --source-commit "$source_commit"

rewrite_archive() {
  local mode="$1" destination="$2"
  python3 - "$archive_one" "$destination" "$mode" "gizclaw-c-sdk-$version" <<'PY'
import gzip
import io
import json
import pathlib
import tarfile
import sys

source_path = pathlib.Path(sys.argv[1])
destination_path = pathlib.Path(sys.argv[2])
mode = sys.argv[3]
root_name = sys.argv[4]
with tarfile.open(source_path, "r:gz") as source:
    members = [(member, source.extractfile(member).read() if member.isreg() else None) for member in source.getmembers()]
archive_mtime = members[0][0].mtime

if mode == "extra":
    info = tarfile.TarInfo(f"{root_name}/unexpected.bin")
    info.mode = 0o644
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.size = 1
    members.append((info, b"x"))
elif mode == "symlink":
    info = tarfile.TarInfo(f"{root_name}/escape")
    info.type = tarfile.SYMTYPE
    info.linkname = "../../escape"
    info.mode = 0o644
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    members.append((info, None))
elif mode == "traversal":
    info = tarfile.TarInfo(f"{root_name}/../escape")
    info.mode = 0o644
    info.uid = info.gid = 0
    info.uname = info.gname = "root"
    info.size = 1
    members.append((info, b"x"))
elif mode == "duplicate":
    member, payload = next(item for item in members if item[0].name == f"{root_name}/MODULE.bazel")
    members.append((member, payload))
elif mode == "mode":
    for member, _ in members:
        if member.name == f"{root_name}/MODULE.bazel":
            member.mode = 0o755
            break
elif mode == "missing":
    members = [item for item in members if item[0].name != f"{root_name}/third_party/nanopb/pb.h"]
elif mode == "provenance":
    changed = []
    for member, payload in members:
        if member.name == f"{root_name}/SOURCE_PROVENANCE.json":
            data = json.loads(payload)
            data["source_commit"] = "2" * 40
            payload = (json.dumps(data, separators=(",", ":"), sort_keys=True) + "\n").encode()
            member.size = len(payload)
        changed.append((member, payload))
    members = changed
else:
    raise SystemExit(f"unknown rewrite mode: {mode}")

with destination_path.open("xb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=archive_mtime) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as target:
            for member, payload in members:
                target.addfile(member, io.BytesIO(payload) if payload is not None else None)
PY
  printf '%s  %s\n' "$(sha256sum "$destination" | awk '{print $1}')" "$(basename "$destination")" >"$destination.sha256"
}

for mode in extra symlink traversal duplicate mode missing provenance; do
  rewritten="$fixture_root/$mode.tar.gz"
  rewrite_archive "$mode" "$rewritten"
  expect_failure "$mode archive" "$repo_root/tools/c-sdk/verify_source_archive.sh" \
    --archive "$rewritten" --version "$version" --source-commit "$source_commit"
done

printf '%s\n' "C SDK source archive contract tests passed"
