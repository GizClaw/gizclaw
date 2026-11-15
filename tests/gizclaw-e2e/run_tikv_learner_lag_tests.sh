#!/usr/bin/env bash
# Fixed public-SDK Learner lag coverage through both Edges, without providers.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if (($# != 0)); then echo 'this entrypoint takes no arguments' >&2; exit 2; fi
exec bash "$script_dir/setup/run_multi_server_suite.sh" native-tikv-lag
