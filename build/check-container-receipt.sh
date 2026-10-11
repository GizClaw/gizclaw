#!/usr/bin/env bash
# Verify delivered bytes, Docker image identity and packaged executable hashes.
set -euo pipefail
[[ $# == 4 || $# == 6 ]] || { echo "usage: $0 RECEIPT TAG SOURCE_COMMIT ASSET_DIR [COMPONENT ARCH]" >&2; exit 2; }
exec python3 "$(dirname "${BASH_SOURCE[0]}")/container_archive.py" receipt "$@"
