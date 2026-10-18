#!/usr/bin/env bash
# Bind the Docker fixture's shared control-plane stores to TiKV.
set -euo pipefail
config_file="${1:?configuration path is required}"
: "${GIZCLAW_E2E_TIKV_PD:?TiKV PD address is required}"
export GIZCLAW_E2E_TIKV_STALE_DELAY="${GIZCLAW_E2E_TIKV_STALE_DELAY:-60s}"
export GIZCLAW_E2E_TIKV_READ_LOCAL_ONLY="${GIZCLAW_E2E_TIKV_READ_LOCAL_ONLY:-false}"
case "$GIZCLAW_E2E_TIKV_READ_LOCAL_ONLY" in true|false) ;; *) echo 'TiKV read-local-only must be true or false' >&2; exit 2 ;; esac
perl -0pi -e '
  s/^storage:\n/storage:\n  shared-tikv:\n    kind: tikv\n    pd_addresses: [$ENV{GIZCLAW_E2E_TIKV_PD}]\n    stale_read_delay: $ENV{GIZCLAW_E2E_TIKV_STALE_DELAY}\n    read_local_only: $ENV{GIZCLAW_E2E_TIKV_READ_LOCAL_ONLY}\n    read_labels:\n      region: local\n/m;
  s/(^  (?:peers|api-keys|friends|friend-groups):\n    kind: keyvalue\n    storage:) memory/$1 shared-tikv/gm;
' "$config_file"
