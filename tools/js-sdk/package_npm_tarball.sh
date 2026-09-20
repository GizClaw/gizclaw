#!/usr/bin/env bash
# Build a deterministic npm package tarball from a clean commit.

set -euo pipefail

package=
version=
source_commit=
source_epoch=
output=
while (($# > 0)); do
  (($# >= 2)) || { echo "missing value for $1" >&2; exit 2; }
  case "$1" in
    --package) package="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    --source-commit) source_commit="$2"; shift 2 ;;
    --source-epoch) source_epoch="$2"; shift 2 ;;
    --output) output="$2"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ "$package" == gizclaw || "$package" == gizclaw-control ]] || { echo "invalid package" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "version must be canonical MAJOR.MINOR.PATCH" >&2; exit 2; }
[[ "$source_commit" =~ ^[0-9a-f]{40}$ ]] || { echo "source commit must be a full lowercase Git SHA" >&2; exit 2; }
[[ "$source_epoch" =~ ^(0|[1-9][0-9]*)$ ]] || { echo "source epoch must be a non-negative integer" >&2; exit 2; }
[[ -n "$output" ]] || { echo "output is required" >&2; exit 2; }
for command_name in git node npm python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
[[ "$(git rev-parse 'HEAD^{commit}')" == "$source_commit" ]] || { echo "source commit is not HEAD" >&2; exit 1; }
[[ "$(git show -s --format=%ct "$source_commit")" == "$source_epoch" ]] || { echo "source epoch does not match source commit" >&2; exit 1; }
# Control's prebuild compiles gizclaw too. Include both SDKs and all build inputs.
owned_paths=(LICENSE package.json package-lock.json .npmrc .gitignore sdk/js tools/js-sdk)
git diff --quiet -- "${owned_paths[@]}" || { echo "JavaScript SDK inputs have unstaged changes" >&2; exit 1; }
git diff --cached --quiet -- "${owned_paths[@]}" || { echo "JavaScript SDK inputs have staged changes" >&2; exit 1; }
[[ -z "$(git ls-files --others --exclude-standard -- "${owned_paths[@]}")" ]] || { echo "JavaScript SDK inputs have untracked files" >&2; exit 1; }

output_dir="$(cd "$(dirname "$output")" 2>/dev/null && pwd)" || { echo "output directory does not exist" >&2; exit 2; }
output="$output_dir/$(basename "$output")"
[[ ! -e "$output" && ! -L "$output" ]] || { echo "archive output already exists" >&2; exit 1; }

stage="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-js-sdk-package.XXXXXX")"
package_complete=false
cleanup() {
  rm -rf "$stage"
  if [[ "$package_complete" != true ]]; then
    rm -f -- "$output"
  fi
}
trap cleanup EXIT

checker="$repo_root/sdk/js/scripts/check-package-release.mjs"
node "$checker" --package "sdk/js/$package"
npm run build --workspace "@gizclaw/$package"
# npm owns the file selection. Run prepack's build explicitly above, then retain
# exactly npm's archive entries; only package.json and tar metadata are rewritten.
npm pack --ignore-scripts --json --workspace "@gizclaw/$package" --pack-destination "$stage" >"$stage/pack.json"
python3 - "$stage" "$package" "$version" <<'PY'
import json
import pathlib
import tarfile
import sys

root = pathlib.Path(sys.argv[1])
package, version = sys.argv[2:]
report = json.loads((root / "pack.json").read_text())
if len(report) != 1 or report[0]["name"] != f"@gizclaw/{package}":
    raise SystemExit("unexpected npm pack identity")
filename = report[0]["filename"]
if pathlib.PurePosixPath(filename).name != filename:
    raise SystemExit("unsafe npm pack filename")
with tarfile.open(root / filename, "r:gz") as archive:
    manifest = json.load(archive.extractfile("package/package.json"))
manifest["version"] = version
manifest.pop("publishConfig", None)
if package == "gizclaw-control":
    manifest["dependencies"]["@gizclaw/gizclaw"] = version
(root / "package.json").write_text(json.dumps(manifest, indent=2) + "\n")
PY
node "$checker" --package "sdk/js/$package" --release-version "$version" --manifest "$stage/package.json"

python3 - "$stage" "$source_epoch" "$output" <<'PY'
import gzip
import io
import json
import pathlib
import tarfile
import sys

root = pathlib.Path(sys.argv[1])
source_epoch = int(sys.argv[2])
output = pathlib.Path(sys.argv[3])
report = json.loads((root / "pack.json").read_text())[0]
with tarfile.open(root / report["filename"], "r:gz") as source:
    members = source.getmembers()
    names = [member.name for member in members]
    if len(names) != len(set(names)):
        raise SystemExit("duplicate npm archive entries")
    packed_files = {member.name for member in members if member.isreg()}
    if packed_files != {"package/" + entry["path"] for entry in report["files"]}:
        raise SystemExit("npm pack file list differs from tarball entries")
    with output.open("xb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, compresslevel=9, mtime=source_epoch) as zipped:
            with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as archive:
                for member in sorted(members, key=lambda item: item.name):
                    pure = pathlib.PurePosixPath(member.name)
                    if (not pure.parts or pure.parts[0] != "package" or ".." in pure.parts
                            or pure.as_posix() != member.name):
                        raise SystemExit(f"unsafe npm archive member: {member.name}")
                    info = tarfile.TarInfo(member.name)
                    info.mtime = source_epoch
                    info.uid = info.gid = 0
                    info.uname = info.gname = "root"
                    if member.isdir():
                        info.type = tarfile.DIRTYPE
                        info.mode = 0o755
                        archive.addfile(info)
                    elif member.isreg():
                        payload = ((root / "package.json").read_bytes() if member.name == "package/package.json"
                                   else source.extractfile(member).read())
                        info.type = tarfile.REGTYPE
                        info.mode = 0o644
                        info.size = len(payload)
                        archive.addfile(info, io.BytesIO(payload))
                    else:
                        raise SystemExit(f"unsupported npm archive member: {member.name}")
PY
"$repo_root/tools/js-sdk/verify_npm_tarball.sh" --archive "$output" --package "$package" --version "$version" --source-epoch "$source_epoch"
package_complete=true
printf '%s\n' "built $output"
