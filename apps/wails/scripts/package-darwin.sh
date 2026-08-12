#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH='' cd -- "$script_dir/../../.." && pwd)
app_dir="$repo_root/apps/wails"
bundle="$app_dir/build/bin/gizclaw-desktop.app"
build_version=${BUILD_VERSION:-dev}
build_commit=${BUILD_COMMIT:-dev}

cd "$app_dir"
wails build -clean "$@"

mkdir -p "$bundle/Contents/Resources"
cd "$repo_root"
go build \
  -ldflags "-X github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo.Version=$build_version -X github.com/GizClaw/gizclaw-go/cmd/internal/buildinfo.Commit=$build_commit" \
  -o "$bundle/Contents/Resources/gizclaw" ./cmd/gizclaw
chmod 755 "$bundle/Contents/Resources/gizclaw"

echo "Packaged $bundle with the GizClaw server companion."
