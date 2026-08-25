#!/usr/bin/env bash
# Build a deterministic standalone C SDK source archive from a clean commit.

set -euo pipefail

version=
source_commit=
source_epoch=
output=
while (($# > 0)); do
  case "$1" in
    --version) version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    --source-epoch) source_epoch="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo "version must be canonical MAJOR.MINOR.PATCH" >&2
  exit 2
}
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "source commit must be a full lowercase Git SHA" >&2; exit 2; }
[[ "$source_epoch" =~ ^(0|[1-9][0-9]*)$ ]] || { echo "source epoch must be a non-negative integer" >&2; exit 2; }
[[ -n "$output" ]] || { echo "output is required" >&2; exit 2; }
for command_name in git python3 sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

repo_root="$(git rev-parse --show-toplevel)"
[[ "$(git rev-parse 'HEAD^{commit}')" == "$source_commit" ]] || { echo "source commit is not HEAD" >&2; exit 1; }
[[ "$(git show -s --format=%ct "$source_commit")" == "$source_epoch" ]] || { echo "source epoch does not match source commit" >&2; exit 1; }

owned_paths=(
  LICENSE
  sdk/c/gizclaw/include
  sdk/c/gizclaw/generated
  sdk/c/gizclaw/src
  sdk/c/gizclaw/tests/gzc_client_smoke_test.c
  sdk/c/gizclaw/tests/gzc_cpp_headers_smoke_test.cpp
  sdk/c/gizclaw/tests/gzc_custom_platform_smoke_test.c
  sdk/c/gizclaw/packaging
  tools/c-sdk
)
git -C "$repo_root" diff --quiet -- "${owned_paths[@]}" || { echo "C SDK source archive inputs have unstaged changes" >&2; exit 1; }
git -C "$repo_root" diff --cached --quiet -- "${owned_paths[@]}" || { echo "C SDK source archive inputs have staged changes" >&2; exit 1; }

nanopb_path="$repo_root/third_party/nanopb/upstream"
nanopb_commit="$(git -C "$repo_root" ls-tree "$source_commit" third_party/nanopb/upstream | awk '{print $3}')"
[[ "$nanopb_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "selected commit has no nanopb gitlink" >&2; exit 1; }
[[ -d "$nanopb_path/.git" || -f "$nanopb_path/.git" ]] || { echo "nanopb submodule is not initialized" >&2; exit 1; }
[[ "$(git -C "$nanopb_path" rev-parse HEAD)" == "$nanopb_commit" ]] || { echo "nanopb checkout does not match selected gitlink" >&2; exit 1; }
[[ -z "$(git -C "$nanopb_path" status --short)" ]] || { echo "nanopb checkout is dirty" >&2; exit 1; }

output_dir="$(cd "$(dirname "$output")" 2>/dev/null && pwd)" || { echo "output directory does not exist" >&2; exit 2; }
output="$output_dir/$(basename "$output")"
sidecar="$output.sha256"
[[ ! -e "$output" && ! -L "$output" && ! -e "$sidecar" && ! -L "$sidecar" ]] || {
  echo "archive output or checksum already exists" >&2
  exit 1
}

stage_parent="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-c-sdk-package.XXXXXX")"
package_complete=false
cleanup() {
  rm -rf "$stage_parent"
  if [[ "$package_complete" != true ]]; then
    rm -f -- "$output" "$sidecar"
  fi
}
trap cleanup EXIT
root_name="gizclaw-c-sdk-$version"
stage="$stage_parent/$root_name"
mkdir -p "$stage/THIRD_PARTY_LICENSES" "$stage/tests" "$stage/third_party/nanopb"

copy_tracked_tree() {
  local source_prefix="$1" destination_prefix="$2" file relative destination
  while IFS= read -r -d '' file; do
    relative="${file#"$source_prefix/"}"
    destination="$stage/$destination_prefix/$relative"
    [[ -f "$repo_root/$file" && ! -L "$repo_root/$file" ]] || { echo "unsupported SDK source input: $file" >&2; exit 1; }
    mkdir -p "$(dirname "$destination")"
    cp "$repo_root/$file" "$destination"
  done < <(git -C "$repo_root" ls-files -z "$source_prefix")
}

copy_tracked_tree sdk/c/gizclaw/include include
copy_tracked_tree sdk/c/gizclaw/generated generated
copy_tracked_tree sdk/c/gizclaw/src src
cp "$repo_root/LICENSE" "$stage/LICENSE"
cp "$repo_root/sdk/c/gizclaw/tests/gzc_client_smoke_test.c" "$stage/tests/"
cp "$repo_root/sdk/c/gizclaw/tests/gzc_cpp_headers_smoke_test.cpp" "$stage/tests/"
cp "$repo_root/sdk/c/gizclaw/tests/gzc_custom_platform_smoke_test.c" "$stage/tests/"
sed "s/@VERSION@/$version/g" "$repo_root/sdk/c/gizclaw/packaging/MODULE.bazel.in" >"$stage/MODULE.bazel"
cp "$repo_root/sdk/c/gizclaw/packaging/BUILD.bazel.in" "$stage/BUILD.bazel"

nanopb_files=(pb.h pb_common.c pb_common.h pb_decode.c pb_decode.h pb_encode.c pb_encode.h)
for file in "${nanopb_files[@]}"; do
  [[ -f "$nanopb_path/$file" && ! -L "$nanopb_path/$file" ]] || { echo "missing nanopb runtime file: $file" >&2; exit 1; }
  cp "$nanopb_path/$file" "$stage/third_party/nanopb/$file"
done
[[ -f "$nanopb_path/LICENSE.txt" && ! -L "$nanopb_path/LICENSE.txt" ]] || { echo "missing nanopb license" >&2; exit 1; }
cp "$nanopb_path/LICENSE.txt" "$stage/THIRD_PARTY_LICENSES/nanopb-LICENSE.txt"

python3 - "$stage/SOURCE_PROVENANCE.json" "$version" "$source_commit" "$source_epoch" "$nanopb_commit" <<'PY'
import json
import pathlib
import sys

path, version, source_commit, source_epoch, nanopb_commit = sys.argv[1:]
payload = {
    "schema_version": 1,
    "repository": "GizClaw/gizclaw",
    "version": version,
    "source_commit": source_commit,
    "source_epoch": int(source_epoch),
    "nanopb_commit": nanopb_commit,
}
pathlib.Path(path).write_text(json.dumps(payload, separators=(",", ":"), sort_keys=True) + "\n", encoding="utf-8")
PY

python3 - "$stage_parent" "$root_name" "$source_epoch" "$output" <<'PY'
import gzip
import pathlib
import tarfile
import sys

stage_parent = pathlib.Path(sys.argv[1])
root_name = sys.argv[2]
source_epoch = int(sys.argv[3])
output = pathlib.Path(sys.argv[4])
root = stage_parent / root_name

with output.open("xb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, compresslevel=9, mtime=source_epoch) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            paths = [root, *sorted(root.rglob("*"), key=lambda item: item.relative_to(stage_parent).as_posix())]
            for path in paths:
                relative = path.relative_to(stage_parent).as_posix()
                info = tarfile.TarInfo(relative)
                info.mtime = source_epoch
                info.uid = 0
                info.gid = 0
                info.uname = "root"
                info.gname = "root"
                if path.is_dir():
                    info.type = tarfile.DIRTYPE
                    info.mode = 0o755
                    archive.addfile(info)
                elif path.is_file() and not path.is_symlink():
                    info.type = tarfile.REGTYPE
                    info.mode = 0o644
                    info.size = path.stat().st_size
                    with path.open("rb") as source:
                        archive.addfile(info, source)
                else:
                    raise SystemExit(f"unsupported archive input: {path}")
PY

printf '%s  %s\n' "$(sha256sum "$output" | awk '{print $1}')" "$(basename "$output")" >"$sidecar"
package_complete=true
printf '%s\n' "built $output"
