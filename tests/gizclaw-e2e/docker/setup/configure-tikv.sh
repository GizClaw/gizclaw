#!/usr/bin/env bash
# Bind the Docker fixture's shared control-plane stores to TiKV.
set -euo pipefail
config_file="${1:?configuration path is required}"
: "${GIZCLAW_E2E_TIKV_PD:?TiKV PD address is required}"
export GIZCLAW_E2E_TIKV_READ_REGION="${GIZCLAW_E2E_TIKV_READ_REGION:-local}"
case "$GIZCLAW_E2E_TIKV_READ_REGION" in local | remote) ;; *) echo 'unsupported fixture read region' >&2; exit 2 ;; esac
perl -0pi -e '
  s/^storage:\n/storage:\n  shared-tikv:\n    kind: tikv\n    pd_addresses: [$ENV{GIZCLAW_E2E_TIKV_PD}]\n    read_labels:\n      region: $ENV{GIZCLAW_E2E_TIKV_READ_REGION}\n/m;
  s/(^  (?:peers|api-keys|friends|friend-groups):\n    kind: keyvalue\n    storage:) memory/$1 shared-tikv/gm;
' "$config_file"
