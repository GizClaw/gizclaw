#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
env_file="$script_dir/.env"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
require_genx_e2e_credentials "$env_file"

cd "$repo_root"
go test -count=1 -v -tags gizclaw_genx_e2e ./tests/genx-e2e/transformer
