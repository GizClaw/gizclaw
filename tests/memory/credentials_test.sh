#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/memory-credentials.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT
env_file="$tmp_dir/.env"
marker="$tmp_dir/must-not-exist"

if require_memory_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
printf 'GIZCLAW_MEMORY_E2E_API_KEY=test-key\n' >"$env_file"
printf 'UNRELATED=$(/usr/bin/touch %s)\n' "$marker" >>"$env_file"
require_memory_e2e_credentials "$env_file"
if [[ -e "$marker" ]]; then exit 1; fi
sed '/^GIZCLAW_MEMORY_E2E_API_KEY=/d' "$env_file" >"$tmp_dir/candidate.env"
if require_memory_e2e_credentials "$tmp_dir/candidate.env" >/dev/null 2>&1; then exit 1; fi
printf 'GIZCLAW_MEMORY_E2E_API_KEY=   \n' >"$env_file"
if require_memory_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
printf 'GIZCLAW_MEMORY_E2E_API_KEY=placeholder\n' >"$env_file"
if require_memory_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
