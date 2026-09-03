#!/usr/bin/env bash
# Validate and compile a standalone C SDK source archive without Bazel.

set -euo pipefail

archive=
version=
source_commit=
while (($# > 0)); do
  case "$1" in
    --archive) archive="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -f "$archive" && ! -L "$archive" ]] || { echo "archive must be a regular file" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid version" >&2; exit 2; }
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "invalid source commit" >&2; exit 2; }
for command_name in ar cc c++ cmp nm python3 sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

archive="$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")"
sidecar="$archive.sha256"
[[ -f "$sidecar" && ! -L "$sidecar" ]] || { echo "archive checksum sidecar is missing" >&2; exit 1; }
expected_sidecar="$(sha256sum "$archive" | awk '{print $1}')  $(basename "$archive")"
cmp -s <(printf '%s\n' "$expected_sidecar") "$sidecar" || { echo "archive checksum sidecar mismatch" >&2; exit 1; }

extract_parent="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-c-sdk-verify.XXXXXX")"
trap 'rm -rf "$extract_parent"' EXIT
root_name="gizclaw-c-sdk-$version"

python3 - "$archive" "$extract_parent" "$root_name" "$version" "$source_commit" <<'PY'
import json
import pathlib
import shutil
import tarfile
import sys

archive_path = pathlib.Path(sys.argv[1])
extract_parent = pathlib.Path(sys.argv[2])
root_name = sys.argv[3]
version = sys.argv[4]
source_commit = sys.argv[5]
seen = set()
directories = set()
files = set()
member_mtime = None

with archive_path.open("rb") as raw:
    header = raw.read(10)
if len(header) != 10 or header[:2] != b"\x1f\x8b":
    raise SystemExit("invalid gzip header")
gzip_mtime = int.from_bytes(header[4:8], "little")

def allowed_file(relative: str) -> bool:
    fixed = {
        "BUILD.bazel",
        "LICENSE",
        "MODULE.bazel",
        "SOURCE_PROVENANCE.json",
        "THIRD_PARTY_LICENSES/nanopb-LICENSE.txt",
        "tests/gzc_client_smoke_test.c",
        "tests/gzc_control_cpp_headers_smoke_test.cpp",
        "tests/gzc_control_smoke_test.c",
        "tests/gzc_cpp_headers_smoke_test.cpp",
        "tests/gzc_custom_platform_smoke_test.c",
    }
    if relative in fixed:
        return True
    path = pathlib.PurePosixPath(relative)
    if path.parts[:1] == ("include",) and path.suffix == ".h":
        return True
    if path.parts[:1] == ("generated",) and path.name.endswith((".pb.c", ".pb.h")):
        return True
    if path.parts[:1] == ("src",) and path.suffix in {".c", ".h"}:
        return True
    if path.parts[:2] == ("control", "include") and path.suffix == ".h":
        return True
    if path.parts[:2] == ("control", "src") and path.suffix in {".c", ".h"}:
        return True
    if path.parts[:2] == ("third_party", "nanopb") and path.name in {
        "pb.h", "pb_common.c", "pb_common.h", "pb_decode.c", "pb_decode.h", "pb_encode.c", "pb_encode.h"
    }:
        return True
    return False

with tarfile.open(archive_path, mode="r:gz") as source:
    members = source.getmembers()
    if not members:
        raise SystemExit("archive is empty")
    if len(members) > 2000 or sum(member.size for member in members) > 32 * 1024 * 1024:
        raise SystemExit("archive exceeds member or uncompressed-size limit")
    for member in members:
        name = member.name.rstrip("/")
        pure = pathlib.PurePosixPath(name)
        if (
            member.name.startswith("/")
            or not pure.parts
            or ".." in pure.parts
            or pure.parts[0] != root_name
            or pure.as_posix() != name
            or member.size > 8 * 1024 * 1024
        ):
            raise SystemExit(f"unsafe archive member: {member.name}")
        if name in seen:
            raise SystemExit(f"duplicate archive member: {member.name}")
        seen.add(name)
        if member_mtime is None:
            member_mtime = member.mtime
        elif member.mtime != member_mtime:
            raise SystemExit(f"non-normalized archive timestamp: {member.name}")
        if not (member.isdir() or member.isreg()):
            raise SystemExit(f"unsupported archive member type: {member.name}")
        expected_mode = 0o755 if member.isdir() else 0o644
        if member.mode != expected_mode or member.uid != 0 or member.gid != 0 or member.uname != "root" or member.gname != "root":
            raise SystemExit(f"non-normalized archive metadata: {member.name}")
        relative = pathlib.PurePosixPath(*pure.parts[1:]).as_posix()
        if member.isreg() and not allowed_file(relative):
            raise SystemExit(f"unexpected archive file: {member.name}")
        if member.isdir():
            directories.add(name)
        else:
            files.add(name)
        destination = extract_parent.joinpath(*pure.parts)
        if member.isdir():
            destination.mkdir(parents=True, exist_ok=True)
        else:
            destination.parent.mkdir(parents=True, exist_ok=True)
            payload = source.extractfile(member)
            if payload is None:
                raise SystemExit(f"could not read archive member: {member.name}")
            with payload, destination.open("xb") as target:
                shutil.copyfileobj(payload, target)

if root_name not in directories:
    raise SystemExit("archive root directory member is missing")
for directory in directories - {root_name}:
    if not any(name.startswith(directory + "/") for name in files):
        raise SystemExit(f"unexpected empty archive directory: {directory}")

root = extract_parent / root_name
required = {
    "BUILD.bazel", "LICENSE", "MODULE.bazel", "SOURCE_PROVENANCE.json",
    "THIRD_PARTY_LICENSES/nanopb-LICENSE.txt", "tests/gzc_client_smoke_test.c",
    "tests/gzc_cpp_headers_smoke_test.cpp", "tests/gzc_custom_platform_smoke_test.c",
    "tests/gzc_control_smoke_test.c", "tests/gzc_control_cpp_headers_smoke_test.cpp",
    "control/include/gzc_control.h", "control/src/gzc_control_api.c",
    "control/src/gzc_control_error.c", "control/src/gzc_control_http.c",
    "control/src/gzc_control_internal.h", "control/src/gzc_control_model.c",
    "src/gzc_platform.c", "third_party/nanopb/pb.h", "third_party/nanopb/pb_common.c",
    "third_party/nanopb/pb_common.h", "third_party/nanopb/pb_decode.c",
    "third_party/nanopb/pb_decode.h", "third_party/nanopb/pb_encode.c",
    "third_party/nanopb/pb_encode.h",
}
missing = sorted(item for item in required if not (root / item).is_file())
if missing:
    raise SystemExit("missing archive files: " + ", ".join(missing))

provenance = json.loads((root / "SOURCE_PROVENANCE.json").read_text(encoding="utf-8"))
if list(sorted(provenance)) != sorted(["nanopb_commit", "repository", "schema_version", "source_commit", "source_epoch", "version"]):
    raise SystemExit("invalid provenance fields")
if provenance.get("schema_version") != 1 or provenance.get("repository") != "GizClaw/gizclaw":
    raise SystemExit("invalid provenance contract")
if provenance.get("version") != version or provenance.get("source_commit") != source_commit:
    raise SystemExit("provenance identity mismatch")
if not isinstance(provenance.get("source_epoch"), int) or provenance["source_epoch"] < 0:
    raise SystemExit("invalid provenance source epoch")
if member_mtime != provenance["source_epoch"] or gzip_mtime != provenance["source_epoch"]:
    raise SystemExit("archive timestamp does not match provenance source epoch")
nanopb_commit = provenance.get("nanopb_commit")
if not isinstance(nanopb_commit, str) or len(nanopb_commit) != 40 or any(ch not in "0123456789abcdef" for ch in nanopb_commit):
    raise SystemExit("invalid provenance nanopb commit")
canonical_provenance = json.dumps(provenance, separators=(",", ":"), sort_keys=True) + "\n"
if (root / "SOURCE_PROVENANCE.json").read_text(encoding="utf-8") != canonical_provenance:
    raise SystemExit("provenance JSON is not canonical")

module = (root / "MODULE.bazel").read_text(encoding="utf-8")
if (
    'name = "gizclaw_c_sdk"' not in module
    or f'version = "{version}"' not in module
    or 'bazel_dep(name = "platforms", version = "1.1.0")' not in module
    or 'bazel_dep(name = "rules_cc", version = "0.2.17")' not in module
):
    raise SystemExit("module identity mismatch")
build = (root / "BUILD.bazel").read_text(encoding="utf-8")
for target in ("gizclaw_core", "default_platform", "gizclaw", "gizclaw_control"):
    if f'name = "{target}"' not in build:
        raise SystemExit(f"missing Bazel target: {target}")
PY

root="$extract_parent/$root_name"
common_flags=(-std=c11 -Wall -Wextra -Werror -I "$root/include" -I "$root/generated" -I "$root/third_party/nanopb")
core_sources=()
while IFS= read -r source; do
  core_sources+=("$source")
done < <(find "$root/src" "$root/generated" -type f -name '*.c' ! -name gzc_platform.c -print | LC_ALL=C sort)
nanopb_sources=("$root/third_party/nanopb/pb_common.c" "$root/third_party/nanopb/pb_decode.c" "$root/third_party/nanopb/pb_encode.c")

cc "${common_flags[@]}" "$root/tests/gzc_client_smoke_test.c" "${core_sources[@]}" \
  "$root/src/gzc_platform.c" "${nanopb_sources[@]}" -o "$extract_parent/default-smoke"
"$extract_parent/default-smoke"

cc "${common_flags[@]}" "$root/tests/gzc_custom_platform_smoke_test.c" "${core_sources[@]}" \
  "${nanopb_sources[@]}" -o "$extract_parent/custom-platform-smoke"
"$extract_parent/custom-platform-smoke"

c++ -std=c++17 -Wall -Wextra -Werror -I "$root/include" -I "$root/generated" -I "$root/third_party/nanopb" \
  "$root/tests/gzc_cpp_headers_smoke_test.cpp" -o "$extract_parent/cpp-headers-smoke"
"$extract_parent/cpp-headers-smoke"

control_sources=()
while IFS= read -r source; do
  control_sources+=("$source")
done < <(find "$root/control/src" -type f -name '*.c' -print | LC_ALL=C sort)
control_flags=("${common_flags[@]}" -I "$root/control/include" -I "$root/control/src")

cc "${control_flags[@]}" "$root/tests/gzc_control_smoke_test.c" "${control_sources[@]}" \
  "$root/src/gzc_buffer.c" "$root/src/gzc_common.c" "$root/src/gzc_json.c" "$root/src/gzc_platform.c" \
  -o "$extract_parent/control-smoke"
"$extract_parent/control-smoke"

c++ -std=c++17 -Wall -Wextra -Werror -I "$root/include" -I "$root/generated" \
  -I "$root/third_party/nanopb" -I "$root/control/include" \
  "$root/tests/gzc_control_cpp_headers_smoke_test.cpp" -o "$extract_parent/control-cpp-headers-smoke"
"$extract_parent/control-cpp-headers-smoke"

if [[ "$(uname -s)" == Linux ]]; then
  cc "${common_flags[@]}" -fsanitize=address,undefined -fno-omit-frame-pointer -g \
    "$root/tests/gzc_client_smoke_test.c" "${core_sources[@]}" "$root/src/gzc_platform.c" \
    "${nanopb_sources[@]}" -o "$extract_parent/sanitizer-smoke"
  ASAN_OPTIONS=detect_leaks=1:halt_on_error=1 UBSAN_OPTIONS=halt_on_error=1:print_stacktrace=1 \
    "$extract_parent/sanitizer-smoke"
fi

cc "${common_flags[@]}" -ffreestanding -fno-builtin -c "$root/src/gzc_event.c" -o "$extract_parent/gzc_event.o"
cc "${common_flags[@]}" -ffreestanding -fno-builtin -c "$root/src/gzc_json.c" -o "$extract_parent/gzc_json.o"
ar rcs "$extract_parent/libgzc_ascii_freestanding.a" "$extract_parent/gzc_event.o" "$extract_parent/gzc_json.o"
if nm -u "$extract_parent/libgzc_ascii_freestanding.a" | grep -E '(_ctype_|__ctype_b_loc|_?isdigit$|_?isspace$)'; then
  echo "C SDK archive imports locale-aware ctype classification" >&2
  exit 1
fi

printf '%s\n' "verified $archive"
