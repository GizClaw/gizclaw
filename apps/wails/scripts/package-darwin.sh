#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/../../.." && pwd)
app_dir="$repo_root/apps/wails"
bundle="$app_dir/build/bin/gizclaw-desktop.app"

cd "$app_dir"
wails build -clean "$@"

mkdir -p "$bundle/Contents/Resources"
cd "$repo_root"
go build -o "$bundle/Contents/Resources/gizclaw" ./cmd/gizclaw
chmod 700 "$bundle/Contents/Resources/gizclaw"

echo "Packaged $bundle with the GizClaw server companion."
