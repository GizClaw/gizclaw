#!/usr/bin/env bash
set -euo pipefail
[[ $# == 1 || $# == 2 ]] || { echo "usage: $0 IMAGE_ID [amd64|arm64]" >&2; exit 2; }
# shellcheck source=common.sh
# shellcheck disable=SC1091
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
gizclaw_image_env "${2:-}"
gizclaw_load_image "$1" "$GIZCLAW_DOCKER_ARCH"
