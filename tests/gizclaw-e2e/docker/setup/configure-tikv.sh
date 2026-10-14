#!/usr/bin/env bash
# Bind the Docker fixture's shared control-plane stores to TiKV.
set -euo pipefail
config_file="${1:?configuration path is required}"
: "${GIZCLAW_E2E_TIKV_PD:?TiKV PD address is required}"
export GIZCLAW_E2E_TIKV_STALE_DELAY="${GIZCLAW_E2E_TIKV_STALE_DELAY:-60s}"
perl -0pi -e '
  s/^storage:\n/storage:\n  shared-tikv:\n    kind: tikv\n    pd_addresses: [$ENV{GIZCLAW_E2E_TIKV_PD}]\n    stale_read_delay: $ENV{GIZCLAW_E2E_TIKV_STALE_DELAY}\n    read_labels:\n      region: local\n/m;
  s/(^  (?:peers|api-keys|friends|friend-groups):\n    kind: keyvalue\n    storage:) memory/$1 shared-tikv/gm;
' "$config_file"
