#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../../.." && pwd)"
bin_dir="$repo_root/tests/gizclaw-e2e/testdata/bin"
bin_path="$bin_dir/gizclaw"

mkdir -p "$bin_dir"
(cd "$repo_root" && go build -o "$bin_path" ./cmd/gizclaw)
echo "$bin_path"
