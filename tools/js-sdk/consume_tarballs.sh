#!/usr/bin/env bash
# Install both Release tarballs outside the workspace and import every public entry.

set -euo pipefail

asset_dir=
version=
while (($# > 0)); do
  (($# >= 2)) || { echo "missing value for $1" >&2; exit 2; }
  case "$1" in
    --asset-dir) asset_dir="$2"; shift 2 ;;
    --version) version="$2"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done
[[ -d "$asset_dir" && ! -L "$asset_dir" ]] || { echo "asset directory must be regular" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid version" >&2; exit 2; }
for command_name in node npm; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
asset_dir="$(cd "$asset_dir" && pwd)"
for package in gizclaw gizclaw-control; do
  "$repo_root/tools/js-sdk/verify_npm_tarball.sh" \
    --archive "$asset_dir/npm-$package-$version.tgz" --package "$package" --version "$version"
done
work="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-js-sdk-consume.XXXXXX")"
trap 'rm -rf "$work"' EXIT
cat >"$work/package.json" <<'JSON'
{"name":"gizclaw-sdk-consumer","private":true,"type":"module"}
JSON
(
  cd "$work"
  # Both direct local dependencies satisfy control's exact transitive dependency.
  npm install --no-audit --no-fund "$asset_dir/npm-gizclaw-$version.tgz" "$asset_dir/npm-gizclaw-control-$version.tgz"
  npm ls @gizclaw/gizclaw @gizclaw/gizclaw-control
  node --input-type=module - "$version" <<'JS'
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
const version = process.argv[2];
const lock = JSON.parse(readFileSync("package-lock.json", "utf8"));
for (const name of ["@gizclaw/gizclaw", "@gizclaw/gizclaw-control"]) {
  const path = `node_modules/${name}`;
  const manifest = JSON.parse(readFileSync(`${path}/package.json`, "utf8"));
  assert.equal(manifest.version, version);
  assert.equal(lock.packages[path].version, version);
  assert.match(lock.packages[path].resolved, /^file:/u);
  for (const entry of Object.keys(manifest.exports)) {
    const imported = await import(entry === "." ? name : name + entry.slice(1));
    assert.ok(Object.keys(imported).length > 0, `${name}/${entry} has no exports`);
  }
}
const { createGizClawControlClient } = await import("@gizclaw/gizclaw-control");
assert.equal(typeof createGizClawControlClient, "function");
console.log(`imported both SDKs at ${version} from local npm tarballs`);
JS
)
printf '%s\n' "installed JavaScript SDK $version tarballs"
