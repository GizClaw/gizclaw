#!/usr/bin/env bash
# Real two-Server Giztest and KV controls with two healthy, differently labelled Learners.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec bash "$script_dir/setup/run_multi_server_suite.sh" tikv-read-routing
