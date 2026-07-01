#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
bin_dir="$repo_root/tests/gizclaw-e2e/testdata/bin"
bin_path="$bin_dir/gizclaw-desktop"

mkdir -p "$bin_dir"
(cd "$repo_root" && npm --prefix apps/wails/frontend run build >/dev/null)
(cd "$repo_root/apps/wails" && go build -o "$bin_path" .)
echo "$bin_path"
