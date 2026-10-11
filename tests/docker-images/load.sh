#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 || $# == 2 ]] || { echo "usage: $0 CATALOG_ID [amd64|arm64]" >&2; exit 2; }
exec python3 "$(dirname "${BASH_SOURCE[0]}")/load.py" load "$@"
