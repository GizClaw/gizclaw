#!/usr/bin/env bash
# Strict sustained-audio Giztest through the isolated real Server/Edge stack.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export GIZCLAW_NETEM_CONTINUITY=1
exec bash "$script_dir/run_audio_netem_tests.sh"
