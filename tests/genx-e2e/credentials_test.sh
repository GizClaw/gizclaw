#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/genx-credentials.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT
env_file="$tmp_dir/.env"
candidate="$tmp_dir/candidate.env"
marker="$tmp_dir/must-not-exist"

if require_genx_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
# shellcheck disable=SC2154
for name in "${genx_e2e_credentials[@]}"; do printf '%s=test-value\n' "$name" >>"$env_file"; done
printf 'UNRELATED=$(/usr/bin/touch %s)\n' "$marker" >>"$env_file"
require_genx_e2e_credentials "$env_file"
if [[ -e "$marker" ]]; then exit 1; fi
for name in "${genx_e2e_credentials[@]}"; do
  grep -v "^${name}=" "$env_file" >"$candidate"
  if require_genx_e2e_credentials "$candidate" >/dev/null 2>&1; then exit 1; fi
done
sed -i.bak '1s/=.*/=   /' "$env_file"
if require_genx_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
sed -i.bak '1s/=.*/=placeholder/' "$env_file"
if require_genx_e2e_credentials "$env_file" >/dev/null 2>&1; then exit 1; fi
