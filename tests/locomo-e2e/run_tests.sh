#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
env_file="$script_dir/.env"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
require_locomo_e2e_credentials "$env_file"

bash "$script_dir/selection.sh" '^TestLoCoMo(FlowcraftBM25SinglePass|FlowcraftHybridSinglePass|FlowcraftHybridTwoPass|Mem0PlatformDefault|Mem0PlatformCustomInstructions|VolcAgentKitDefault)$'
