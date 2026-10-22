#!/usr/bin/env bash
# Validate the structure and identity of one pub-hosted Flutter SDK package archive.

set -euo pipefail

archive=
package=
version=
while (($# > 0)); do
  case "$1" in
    --archive) archive="${2:-}"; shift 2 ;;
    --package) package="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -f "$archive" && ! -L "$archive" ]] || { echo "archive must be a regular file" >&2; exit 2; }
[[ "$package" == gizclaw || "$package" == gizclaw_control ]] || { echo "invalid package" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid version" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "required command not found: python3" >&2; exit 2; }
python3 -c 'import yaml' 2>/dev/null || { echo "required Python module not found: yaml" >&2; exit 2; }

python3 - "$archive" "$package" "$version" <<'PY'
import pathlib
import re
import tarfile
import sys
from urllib.parse import urlsplit

import yaml

archive_path = pathlib.Path(sys.argv[1])
package = sys.argv[2]
version = sys.argv[3]

with archive_path.open("rb") as raw:
    header = raw.read(10)
if len(header) != 10 or header[:2] != b"\x1f\x8b":
    raise SystemExit("invalid gzip header")
gzip_mtime = int.from_bytes(header[4:8], "little")

seen = set()
directories = set()
files = {}
member_mtime = None
with tarfile.open(archive_path, mode="r:gz") as source:
    members = source.getmembers()
    if not members:
        raise SystemExit("archive is empty")
    if len(members) > 2000 or sum(member.size for member in members) > 32 * 1024 * 1024:
        raise SystemExit("archive exceeds member or uncompressed-size limit")
    for member in members:
        name = member.name
        pure = pathlib.PurePosixPath(name)
        if (
            name.startswith(("/", "./"))
            or not pure.parts
            or ".." in pure.parts
            or pure.as_posix() != name
            or member.size > 8 * 1024 * 1024
        ):
            raise SystemExit(f"unsafe archive member: {name}")
        if name in seen:
            raise SystemExit(f"duplicate archive member: {name}")
        seen.add(name)
        if member_mtime is None:
            member_mtime = member.mtime
        elif member.mtime != member_mtime:
            raise SystemExit(f"non-normalized archive timestamp: {name}")
        if not (member.isdir() or member.isreg()):
            raise SystemExit(f"unsupported archive member type: {name}")
        expected_mode = 0o755 if member.isdir() else 0o644
        if member.mode != expected_mode or member.uid != 0 or member.gid != 0 or member.uname != "root" or member.gname != "root":
            raise SystemExit(f"non-normalized archive metadata: {name}")
        if member.isdir():
            if pure.parts[0] != "lib":
                raise SystemExit(f"unexpected archive directory: {name}")
            directories.add(name)
            continue
        if not (name in {"LICENSE", "pubspec.yaml"} or (pure.parts[0] == "lib" and pure.suffix == ".dart")):
            raise SystemExit(f"unexpected archive file: {name}")
        payload = source.extractfile(member)
        if payload is None:
            raise SystemExit(f"could not read archive member: {name}")
        with payload:
            files[name] = payload.read()

if gzip_mtime != member_mtime:
    raise SystemExit("gzip timestamp does not match archive members")
for directory in directories:
    if not any(name.startswith(directory + "/") for name in files):
        raise SystemExit(f"unexpected empty archive directory: {directory}")
for name in files:
    parent = pathlib.PurePosixPath(name).parent
    while parent.as_posix() != ".":
        if parent.as_posix() not in directories:
            raise SystemExit(f"missing parent directory member: {parent}")
        parent = parent.parent
library = f"lib/{package}.dart"
missing = sorted(name for name in ("LICENSE", "pubspec.yaml", library) if name not in files)
if missing:
    raise SystemExit("missing archive files: " + ", ".join(missing))

pubspec = files["pubspec.yaml"].decode("utf-8").splitlines()
if pubspec[:1] != [f"name: {package}"]:
    raise SystemExit("pubspec package name mismatch")
if [line for line in pubspec if line.startswith("version:")] != [f"version: {version}"]:
    raise SystemExit("pubspec version mismatch")
if not any(line.startswith("environment:") for line in pubspec):
    raise SystemExit("pubspec has no SDK environment")
class UniqueKeyLoader(yaml.SafeLoader):
    pass


def unique_mapping(loader, node):
    result = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node)
        if not isinstance(key, str) or key in result:
            raise ValueError("duplicate or non-string pubspec key")
        result[key] = loader.construct_object(value_node)
    return result


UniqueKeyLoader.add_constructor(
    yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG, unique_mapping
)
try:
    manifest = yaml.load("\n".join(pubspec), Loader=UniqueKeyLoader)
except (yaml.YAMLError, ValueError, TypeError, RecursionError):
    raise SystemExit("pubspec YAML is invalid")
if not isinstance(manifest, dict):
    raise SystemExit("pubspec must be a mapping")
if any(key in manifest for key in ("dependency_overrides", "workspace", "resolution")):
    raise SystemExit("pubspec contains a local-only resolution field")
for section in ("dependencies", "dev_dependencies"):
    dependencies = manifest.get(section, {})
    if not isinstance(dependencies, dict):
        raise SystemExit("pubspec dependencies must be a mapping")
    for description in dependencies.values():
        if not isinstance(description, dict):
            continue
        if "path" in description:
            raise SystemExit("pubspec contains a path dependency")
        if "git" not in description:
            continue
        git = description["git"]
        if (
            set(description) - {"git", "version"}
            or not isinstance(git, dict)
            or set(git) != {"url", "ref"}
            or not isinstance(git["url"], str)
            or not isinstance(git["ref"], str)
            or not re.fullmatch(r"[0-9a-f]{40}", git["ref"])
            or any(character.isspace() for character in git["url"])
        ):
            raise SystemExit("pubspec Git dependencies require an immutable HTTPS revision")
        try:
            remote = urlsplit(git["url"])
        except ValueError:
            raise SystemExit("pubspec Git dependency URL is invalid")
        if (
            remote.scheme != "https" or not remote.hostname
            or remote.username is not None or remote.password is not None
            or remote.query or remote.fragment
        ):
            raise SystemExit("pubspec Git dependencies require a credential-free HTTPS URL")
PY

printf '%s\n' "verified $archive"
