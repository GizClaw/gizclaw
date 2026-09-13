#!/bin/sh
# Flutter sanitizes hook environments. Record the explicit checkout in ignored
# package metadata, then run Flutter from the caller's directory. Example:
# GIZOS_ROOT=/path/to/gizos ./tool/with_gizos.sh test
set -eu
: "${GIZOS_ROOT:?Set GIZOS_ROOT to your GizOS checkout}"
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$package_dir/.dart_tool"
printf '%s\n' "$GIZOS_ROOT" > "$package_dir/.dart_tool/gizos-root"
exec flutter "$@"
