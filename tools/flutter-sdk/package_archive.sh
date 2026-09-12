#!/usr/bin/env bash
# Build a deterministic pub-hosted archive for one Flutter SDK package from a clean commit.

set -euo pipefail

package=
version=
source_commit=
source_epoch=
output=
while (($# > 0)); do
  case "$1" in
    --package) package="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    --source-commit) source_commit="${2:-}"; shift 2 ;;
    --source-epoch) source_epoch="${2:-}"; shift 2 ;;
    --output) output="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ "$package" == gizclaw || "$package" == gizclaw_control ]] || {
  echo "package must be gizclaw or gizclaw_control" >&2
  exit 2
}
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || {
  echo "version must be canonical MAJOR.MINOR.PATCH" >&2
  exit 2
}
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "source commit must be a full lowercase Git SHA" >&2; exit 2; }
[[ "$source_epoch" =~ ^(0|[1-9][0-9]*)$ ]] || { echo "source epoch must be a non-negative integer" >&2; exit 2; }
[[ -n "$output" ]] || { echo "output is required" >&2; exit 2; }
for command_name in git python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

repo_root="$(git rev-parse --show-toplevel)"
[[ "$(git rev-parse 'HEAD^{commit}')" == "$source_commit" ]] || { echo "source commit is not HEAD" >&2; exit 1; }
[[ "$(git show -s --format=%ct "$source_commit")" == "$source_epoch" ]] || { echo "source epoch does not match source commit" >&2; exit 1; }

package_root="sdk/flutter/$package"
owned_paths=(LICENSE "$package_root/pubspec.yaml" "$package_root/lib" tools/flutter-sdk)
git -C "$repo_root" diff --quiet -- "${owned_paths[@]}" || { echo "Flutter SDK archive inputs have unstaged changes" >&2; exit 1; }
git -C "$repo_root" diff --cached --quiet -- "${owned_paths[@]}" || { echo "Flutter SDK archive inputs have staged changes" >&2; exit 1; }

output_dir="$(cd "$(dirname "$output")" 2>/dev/null && pwd)" || { echo "output directory does not exist" >&2; exit 2; }
output="$output_dir/$(basename "$output")"
[[ ! -e "$output" && ! -L "$output" ]] || { echo "archive output already exists" >&2; exit 1; }

stage="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-flutter-sdk-package.XXXXXX")"
package_complete=false
cleanup() {
  rm -rf "$stage"
  if [[ "$package_complete" != true ]]; then
    rm -f -- "$output"
  fi
}
trap cleanup EXIT

# Pub extracts hosted archives in place, so the package root is the archive root.
file_count=0
while IFS= read -r -d '' file; do
  relative="${file#"$package_root/"}"
  [[ -f "$repo_root/$file" && ! -L "$repo_root/$file" ]] || { echo "unsupported SDK source input: $file" >&2; exit 1; }
  [[ "$relative" == *.dart ]] || { echo "unexpected non-Dart library input: $file" >&2; exit 1; }
  mkdir -p "$(dirname "$stage/$relative")"
  cp "$repo_root/$file" "$stage/$relative"
  file_count=$((file_count + 1))
done < <(git -C "$repo_root" ls-files -z "$package_root/lib")
((file_count > 0)) || { echo "package has no tracked library files: $package" >&2; exit 1; }
cp "$repo_root/LICENSE" "$stage/LICENSE"

# The Release version replaces the development version so every GizClaw tag owns
# one immutable hosted package version.
python3 - "$repo_root/$package_root/pubspec.yaml" "$stage/pubspec.yaml" "$package" "$version" <<'PY'
import pathlib
import sys

source, destination, package, version = sys.argv[1:]
lines = pathlib.Path(source).read_text(encoding="utf-8").splitlines(keepends=True)
if not lines or lines[0].rstrip("\n") != f"name: {package}":
    raise SystemExit("pubspec must start with the exact package name")
version_lines = [index for index, line in enumerate(lines) if line.startswith("version:")]
if len(version_lines) != 1:
    raise SystemExit("pubspec must declare exactly one top-level version")
lines[version_lines[0]] = f"version: {version}\n"
pathlib.Path(destination).write_text("".join(lines), encoding="utf-8")
PY

python3 - "$stage" "$source_epoch" "$output" <<'PY'
import gzip
import pathlib
import tarfile
import sys

root = pathlib.Path(sys.argv[1])
source_epoch = int(sys.argv[2])
output = pathlib.Path(sys.argv[3])

with output.open("xb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, compresslevel=9, mtime=source_epoch) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            for path in sorted(root.rglob("*"), key=lambda item: item.relative_to(root).as_posix()):
                info = tarfile.TarInfo(path.relative_to(root).as_posix())
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

package_complete=true
printf '%s\n' "built $output"
