#!/usr/bin/env bash
# Regression tests for deterministic npm tarballs and their rejection paths.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
for command_name in cmp git node npm python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done

fixture_root="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-js-sdk-contract.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT
version=0.18.17
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
  "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
    --package "$1" --version "$version" --source-commit "$source_commit" --source-epoch "$source_epoch" --output "$2"
}

for package in gizclaw gizclaw-control; do
  archive_one="$fixture_root/npm-$package-$version.tgz"
  archive_two="$fixture_root/npm-$package-$version-b.tgz"
  package_archive "$package" "$archive_one"
  package_archive "$package" "$archive_two"
  cmp "$archive_one" "$archive_two"
  npm pack --dry-run --ignore-scripts --json --workspace "@gizclaw/$package" >"$fixture_root/pack.json"
  python3 - "$archive_one" "$fixture_root/pack.json" <<'PYTHON'
import json
import tarfile
import sys

with open(sys.argv[2]) as report:
    expected = {"package/" + item["path"] for item in json.load(report)[0]["files"]}
with tarfile.open(sys.argv[1], "r:gz") as archive:
    actual = {member.name for member in archive.getmembers() if member.isreg()}
if actual != expected:
    raise SystemExit("Release tarball file set differs from npm pack")
PYTHON
  "$repo_root/tools/js-sdk/verify_npm_tarball.sh" --archive "$archive_one" --package "$package" --version "$version"
  expect_failure "$package archive for another package" "$repo_root/tools/js-sdk/verify_npm_tarball.sh" \
    --archive "$archive_one" --package "$([[ "$package" == gizclaw ]] && echo gizclaw-control || echo gizclaw)" \
    --version "$version"
  expect_failure "$package archive for another version" "$repo_root/tools/js-sdk/verify_npm_tarball.sh" \
    --archive "$archive_one" --package "$package" --version 0.0.1
done

archive="$fixture_root/npm-gizclaw-control-$version.tgz"
expect_failure "unknown package" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package giztest --version "$version" --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tgz"
expect_failure "invalid version" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package gizclaw --version 00.0.0 --source-commit "$source_commit" --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tgz"
expect_failure "short source commit" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package gizclaw --version "$version" --source-commit 1111111 --source-epoch "$source_epoch" \
  --output "$fixture_root/invalid.tgz"
expect_failure "mismatched source epoch" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package gizclaw --version "$version" --source-commit "$source_commit" --source-epoch 1 \
  --output "$fixture_root/invalid.tgz"
expect_failure "archive overwrite" package_archive gizclaw-control "$archive"
expect_failure "source commit is not HEAD" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package gizclaw --version "$version" --source-commit 1111111111111111111111111111111111111111 \
  --source-epoch "$source_epoch" --output "$fixture_root/invalid.tgz"
expect_failure "negative source epoch" "$repo_root/tools/js-sdk/package_npm_tarball.sh" \
  --package gizclaw --version "$version" --source-commit "$source_commit" \
  --source-epoch -1 --output "$fixture_root/invalid.tgz"
expect_failure "missing argument" "$repo_root/tools/js-sdk/package_npm_tarball.sh" --package
[[ ! -e "$fixture_root/invalid.tgz" ]] || { echo "failed packaging left an output" >&2; exit 1; }
"$repo_root/tools/js-sdk/consume_tarballs.sh" --asset-dir "$fixture_root" --version "$version"

# A separate tiny repository exercises provenance guards and cleanup without
# mutating the caller's worktree or needing another dependency installation.
input_repo="$fixture_root/input-repo"
mkdir -p "$input_repo/tools/js-sdk" "$input_repo/sdk/js/scripts" "$input_repo/sdk/js/gizclaw" "$fixture_root/stages"
cp "$repo_root/tools/js-sdk/package_npm_tarball.sh" "$input_repo/tools/js-sdk/"
cp "$repo_root/sdk/js/scripts/check-package-release.mjs" "$input_repo/sdk/js/scripts/"
node --input-type=module - "$repo_root" "$input_repo" <<'JS'
import { writeFileSync } from "node:fs";
import { pathToFileURL } from "node:url";
const [source, root] = process.argv.slice(2);
const { DEVELOPMENT_VERSION } = await import(pathToFileURL(`${source}/sdk/js/scripts/check-package-release.mjs`));
writeFileSync(`${root}/package.json`, JSON.stringify({ private: true, workspaces: ["sdk/js/gizclaw"] }));
writeFileSync(`${root}/sdk/js/gizclaw/package.json`, JSON.stringify({
  name: "@gizclaw/gizclaw", version: DEVELOPMENT_VERSION, scripts: { build: "exit 19" },
  publishConfig: { registry: "https://npm.pkg.github.com" },
}));
writeFileSync(`${root}/package-lock.json`, JSON.stringify({ packages: {
  "sdk/js/gizclaw": { version: DEVELOPMENT_VERSION },
} }));
JS
(
  cd "$input_repo"
  git init -q
  git add .
  git -c user.name='SDK Contract Test' -c user.email=test@example.com -c commit.gpgsign=false commit -qm 'test: define packaging inputs'
)
input_commit="$(git -C "$input_repo" rev-parse HEAD)"
input_epoch="$(git -C "$input_repo" show -s --format=%ct HEAD)"
package_input_fixture() {
  TMPDIR="$fixture_root/stages" "$input_repo/tools/js-sdk/package_npm_tarball.sh" \
    --package gizclaw --version "$version" --source-commit "$input_commit" \
    --source-epoch "$input_epoch" --output "$fixture_root/failed-build.tgz"
}
for state in unstaged staged untracked; do
  if [[ "$state" == untracked ]]; then
    touch "$input_repo/sdk/js/gizclaw/untracked.ts"
  else
    printf '\n' >>"$input_repo/sdk/js/gizclaw/package.json"
    if [[ "$state" == staged ]]; then
      git -C "$input_repo" add sdk/js/gizclaw/package.json
    fi
  fi
  expect_failure "$state owned input" package_input_fixture
  grep -Fq "inputs have $state" "$fixture_root/failure.stderr"
  git -C "$input_repo" restore --source=HEAD --staged --worktree sdk/js/gizclaw/package.json
  rm -f "$input_repo/sdk/js/gizclaw/untracked.ts"
done
expect_failure "build failure cleanup" package_input_fixture
grep -Fq 'exit 19' "$fixture_root/failure.stdout"
# npm may create its own node-compile-cache under TMPDIR; check owned staging.
[[ ! -e "$fixture_root/failed-build.tgz" && -z "$(find "$fixture_root/stages" -name 'gizclaw-js-sdk-package.*' -print)" ]] || {
  echo "failed packaging left output or staging files" >&2
  exit 1
}

rewrite_archive() {
  local mode="$1" destination="$2"
  python3 - "$archive" "$destination" "$mode" <<'PY'
import gzip
import io
import json
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


if mode == "root-directory":
    for member, _ in members:
        member.name = "wrong/" + member.name
elif mode == "symlink":
    members.append(file_member("package/escape.js", None, type=tarfile.SYMTYPE, linkname="../../escape"))
elif mode == "traversal":
    members.append(file_member("package/../escape.js", b"x"))
elif mode == "duplicate":
    members.append(next(item for item in members if item[0].name == "package/package.json"))
elif mode == "mode":
    next(member for member, _ in members if member.name == "package/package.json").mode = 0o755
elif mode == "timestamp":
    next(member for member, _ in members if member.name == "package/package.json").mtime = archive_mtime + 1
elif mode in {"missing-js", "missing-types"}:
    missing = "package/dist/index.js" if mode == "missing-js" else "package/dist/index.d.ts"
    members = [item for item in members if item[0].name != missing]
elif mode in {"version", "dependency", "publish-config", "export"}:
    changed = []
    for member, payload in members:
        if member.name == "package/package.json":
            manifest = json.loads(payload)
            if mode == "version":
                manifest["version"] = "0.18.18"
            elif mode == "dependency":
                manifest["dependencies"]["@gizclaw/gizclaw"] = "^0.18.17"
            elif mode == "publish-config":
                manifest["publishConfig"] = {"access": "public"}
            else:
                manifest["exports"]["."]["import"] = "./dist/absent.js"
            payload = json.dumps(manifest).encode()
            member.size = len(payload)
        changed.append((member, payload))
    members = changed
elif mode not in {"order", "gzip-time", "pax"}:
    raise SystemExit(f"unknown rewrite mode: {mode}")
members.sort(key=lambda item: item[0].name)
if mode == "order":
    members.reverse()

with open(destination_path, "xb") as raw:
    with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=archive_mtime + (1 if mode == "gzip-time" else 0)) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.PAX_FORMAT if mode == "pax" else tarfile.USTAR_FORMAT) as target:
            if mode == "pax":
                members[0][0].pax_headers = {"comment": "not USTAR"}
            for member, payload in members:
                target.addfile(member, io.BytesIO(payload) if payload is not None else None)
PY
}

for mode in root-directory symlink traversal duplicate mode timestamp missing-js missing-types version dependency publish-config export order gzip-time pax; do
  rewritten="$fixture_root/$mode.tgz"
  rewrite_archive "$mode" "$rewritten"
  expect_failure "$mode archive" "$repo_root/tools/js-sdk/verify_npm_tarball.sh" \
    --archive "$rewritten" --package gizclaw-control --version "$version"
done

printf '%s\n' "JavaScript SDK archive contract tests passed"
