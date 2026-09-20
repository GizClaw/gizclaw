#!/usr/bin/env bash
# Validate npm archive structure, normalized metadata, exports and Release identity.

set -euo pipefail

archive=
package=
version=
source_epoch=
while (($# > 0)); do
  (($# >= 2)) || { echo "missing value for $1" >&2; exit 2; }
  case "$1" in
    --archive) archive="$2"; shift 2 ;;
    --package) package="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    --source-epoch) source_epoch="$2"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
[[ -f "$archive" && ! -L "$archive" ]] || { echo "archive must be a regular file" >&2; exit 2; }
[[ "$package" == gizclaw || "$package" == gizclaw-control ]] || { echo "invalid package" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid version" >&2; exit 2; }
[[ -z "$source_epoch" || "$source_epoch" =~ ^(0|[1-9][0-9]*)$ ]] || { echo "invalid source epoch" >&2; exit 2; }
for command_name in node python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-js-sdk-verify.XXXXXX")"
trap 'rm -rf "$work"' EXIT

python3 - "$archive" "$package" "$source_epoch" "$work/package.json" <<'PY'
import json
import pathlib
import tarfile
import sys

archive_path, package, source_epoch, manifest_path = sys.argv[1:]
with open(archive_path, "rb") as raw:
    header = raw.read(10)
if len(header) != 10 or header[:4] != b"\x1f\x8b\x08\x00":
    raise SystemExit("invalid or non-normalized gzip header")
gzip_mtime = int.from_bytes(header[4:8], "little")
if source_epoch and gzip_mtime != int(source_epoch):
    raise SystemExit("gzip timestamp does not match source epoch")
seen = set()
files = {}
with tarfile.open(archive_path, "r:gz") as source:
    members = source.getmembers()
    if not members or len(members) > 4000 or sum(member.size for member in members) > 64 * 1024 * 1024:
        raise SystemExit("archive is empty or exceeds member/size limit")
    if [member.name for member in members] != sorted(member.name for member in members):
        raise SystemExit("archive entries are not sorted")
    for member in members:
        name = member.name
        pure = pathlib.PurePosixPath(name)
        if (not pure.parts or pure.parts[0] != "package" or ".." in pure.parts
                or pure.as_posix() != name or member.size > 8 * 1024 * 1024):
            raise SystemExit(f"unsafe archive member: {name}")
        if name in seen:
            raise SystemExit(f"duplicate archive member: {name}")
        seen.add(name)
        if not (member.isdir() or member.isreg()) or (name == "package" and not member.isdir()):
            raise SystemExit(f"unsupported archive member: {name}")
        source.fileobj.seek(member.offset)
        if source.fileobj.read(512)[257:265] != b"ustar\x0000" or member.pax_headers:
            raise SystemExit(f"archive is not USTAR: {name}")
        if (member.mtime != gzip_mtime or member.uid != 0 or member.gid != 0
                or member.uname != "root" or member.gname != "root"
                or member.mode != (0o755 if member.isdir() else 0o644)):
            raise SystemExit(f"non-normalized archive metadata: {name}")
        if member.isreg():
            files[name] = source.extractfile(member).read()
required = ["package/package.json", "package/dist/index.js", "package/dist/index.d.ts"]
if package == "gizclaw":
    for entry in ("admin", "rpc", "peerhttp", "signaling", "events", "telemetry"):
        required.extend((f"package/dist/{entry}.js", f"package/dist/{entry}.d.ts"))
for name in required:
    if not files.get(name):
        raise SystemExit(f"missing or empty required file: {name}")
manifest = json.loads(files["package/package.json"])
exports = manifest.get("exports")
if not isinstance(exports, dict) or "." not in exports:
    raise SystemExit("missing package exports")
for entry in exports.values():
    if not isinstance(entry, dict):
        raise SystemExit("invalid package export")
    for condition, suffix in (("types", ".d.ts"), ("import", ".js")):
        target = entry.get(condition)
        if (not isinstance(target, str) or not target.startswith("./dist/")
                or not target.endswith(suffix) or not files.get("package/" + target[2:])):
            raise SystemExit(f"missing {condition} export target: {target}")
pathlib.Path(manifest_path).write_bytes(files["package/package.json"])
PY
# One implementation owns the version equality and exact internal dependency assertions.
node "$repo_root/sdk/js/scripts/check-package-release.mjs" \
  --package "sdk/js/$package" --release-version "$version" --manifest "$work/package.json"
printf '%s\n' "verified $archive"
