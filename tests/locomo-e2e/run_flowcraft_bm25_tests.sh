#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=credentials.sh
source "$script_dir/credentials.sh"
require_locomo_e2e_credentials "$script_dir/.env"
bash "$script_dir/selection.sh" '^TestLoCoMoFlowcraftBM25SinglePass$'
