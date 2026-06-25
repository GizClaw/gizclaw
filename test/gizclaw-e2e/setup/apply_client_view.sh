#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
e2e_dir="$repo_root/test/gizclaw-e2e"
testdata_dir="$e2e_dir/testdata"
bin_path="$testdata_dir/bin/gizclaw"
env_file="${GIZCLAW_E2E_ENV:-$e2e_dir/.env}"

usage() {
  echo "usage: $0 <peer-public-key> [view-name]" >&2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
  exit 2
fi

peer_public_key="$1"
view_name="${2:-default-client}"

if [[ ! "$peer_public_key" =~ ^[1-9A-HJ-NP-Za-km-z]+$ ]]; then
  echo "peer public key must be a non-empty base58 string" >&2
  exit 2
fi
if [[ ! "$view_name" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "view name must be a non-empty safe identifier" >&2
  exit 2
fi

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi

admin_setup_config_home="${GIZCLAW_E2E_ADMIN_SETUP_CONFIG_HOME:-$testdata_dir/admin-config-home}"
admin_setup_context="${GIZCLAW_E2E_ADMIN_SETUP_CONTEXT:-e2e-admin}"

if [[ ! -x "$bin_path" ]]; then
  "$script_dir/build.sh" >/dev/null
fi

resource_file="$(mktemp "${TMPDIR:-/tmp}/gizclaw-client-view.XXXXXX.json")"
trap 'rm -f "$resource_file"' EXIT

cat >"$resource_file" <<JSON
{
  "apiVersion": "gizclaw.admin/v1alpha1",
  "kind": "PeerConfig",
  "metadata": {
    "name": "$peer_public_key"
  },
  "spec": {
    "view": "$view_name"
  }
}
JSON

XDG_CONFIG_HOME="$admin_setup_config_home" \
  "$bin_path" admin apply --context "$admin_setup_context" -f "$resource_file"

echo "applied view '$view_name' to peer '$peer_public_key'"
