#!/usr/bin/env bash
# Install both Flutter SDK archives as ordinary hosted pub dependencies from a
# local static repository and analyze a consumer that imports them.

set -euo pipefail

asset_dir=
version=
while (($# > 0)); do
  case "$1" in
    --asset-dir) asset_dir="${2:-}"; shift 2 ;;
    --version) version="${2:-}"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

[[ -d "$asset_dir" && ! -L "$asset_dir" ]] || { echo "asset directory must be regular" >&2; exit 2; }
[[ "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] || { echo "invalid version" >&2; exit 2; }
for command_name in flutter python3; do
  command -v "$command_name" >/dev/null 2>&1 || { echo "required command not found: $command_name" >&2; exit 2; }
done
python3 -c 'import yaml' 2>/dev/null || { echo "required Python module not found: yaml" >&2; exit 2; }

asset_dir="$(cd "$asset_dir" && pwd)"
packages=(gizclaw gizclaw_control)
for package in "${packages[@]}"; do
  [[ -f "$asset_dir/flutter-$package-$version.tar.gz" ]] || { echo "missing archive for $package" >&2; exit 1; }
done

work="$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/gizclaw-flutter-sdk-consume.XXXXXX")"
server_pid=
cleanup() {
  if [[ -n "$server_pid" ]]; then
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT
repository="$work/repository"
mkdir -p "$repository/api/packages"

cat >"$work/server.py" <<'PY'
import http.server
import os
import pathlib
import sys


class Handler(http.server.SimpleHTTPRequestHandler):
    def guess_type(self, path):
        if "/api/packages/" in path:
            return "application/vnd.pub.v2+json"
        return "application/octet-stream"

    def log_message(self, format, *args):
        sys.stderr.write((format % args) + "\n")


os.chdir(sys.argv[1])
server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
pathlib.Path(sys.argv[2]).write_text(str(server.server_address[1]), encoding="utf-8")
server.serve_forever()
PY
python3 "$work/server.py" "$repository" "$work/port" 2>"$work/server.log" &
server_pid=$!
for _ in $(seq 1 50); do
  [[ -s "$work/port" ]] && break
  sleep 0.1
done
[[ -s "$work/port" ]] || { echo "local pub repository did not start" >&2; cat "$work/server.log" >&2; exit 1; }
hosted_url="http://127.0.0.1:$(cat "$work/port")"

# This mirrors the static Hosted Pub Repository Specification v2 layout that a
# production object store serves: one listing per package plus immutable archives.
for package in "${packages[@]}"; do
  archive_key="packages/$package/versions/$version.tar.gz"
  mkdir -p "$(dirname "$repository/$archive_key")"
  cp "$asset_dir/flutter-$package-$version.tar.gz" "$repository/$archive_key"
  python3 - "$repository/$archive_key" "$repository/api/packages/$package" "$package" "$version" "$hosted_url/$archive_key" <<'PY'
import hashlib
import json
import pathlib
import tarfile
import sys

import yaml

archive, listing, package, version, archive_url = sys.argv[1:]
with tarfile.open(archive, "r:gz") as source:
    pubspec = yaml.safe_load(source.extractfile("pubspec.yaml").read())
if pubspec.get("name") != package or pubspec.get("version") != version:
    raise SystemExit("archive pubspec identity mismatch")
entry = {
    "version": version,
    "archive_url": archive_url,
    "archive_sha256": hashlib.sha256(pathlib.Path(archive).read_bytes()).hexdigest(),
    "pubspec": pubspec,
}
pathlib.Path(listing).write_text(json.dumps({"name": package, "latest": entry, "versions": [entry]}), encoding="utf-8")
PY
done

consumer="$work/consumer"
mkdir -p "$consumer/lib"
cat >"$consumer/pubspec.yaml" <<EOF
name: gizclaw_sdk_consumer
publish_to: none

environment:
  sdk: ^3.10.0

dependencies:
  flutter:
    sdk: flutter
  gizclaw:
    hosted: $hosted_url
    version: $version
  gizclaw_control:
    hosted: $hosted_url
    version: $version
EOF
cat >"$consumer/lib/main.dart" <<'EOF'
import 'package:gizclaw/gizclaw.dart';
import 'package:gizclaw_control/gizclaw_control.dart';

void main() {
  print([GizClawClient, GizClawControlClient]);
}
EOF

(
  cd "$consumer"
  export PUB_CACHE="$work/pub-cache"
  flutter pub get
  flutter analyze --no-pub --no-fatal-infos lib
)

python3 - "$consumer/pubspec.lock" "$hosted_url" "$version" "${packages[@]}" <<'PY'
import sys

import yaml

lock_path, hosted_url, version, *packages = sys.argv[1:]
with open(lock_path, encoding="utf-8") as source:
    lock = yaml.safe_load(source)["packages"]
for package in packages:
    entry = lock.get(package)
    if not entry or entry.get("source") != "hosted" or entry.get("version") != version:
        raise SystemExit(f"{package} was not resolved as hosted {version}")
    if entry.get("description", {}).get("url") != hosted_url:
        raise SystemExit(f"{package} was not resolved from the local repository")
PY

printf '%s\n' "installed Flutter SDK $version archives as hosted pub dependencies"
