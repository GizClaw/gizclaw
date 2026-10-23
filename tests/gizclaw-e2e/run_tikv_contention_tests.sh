#!/usr/bin/env bash
# Provider-free concurrent Server admission against real Docker TiKV.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GIZCLAW_E2E_TIKV_CONTENTION_ONLY=1
exec bash "$script_dir/run_multi_server_tests.sh" "$@"
