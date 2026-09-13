#!/bin/sh
# Flutter sanitizes hook environments; forward the explicit source package path
# through ignored package metadata. Run from the desired Flutter project.
set -eu
: "${GIZOS_LUA_RUNTIME_SRC:?Set GIZOS_LUA_RUNTIME_SRC to a .tar.gz or extracted runtime source directory}"
package_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
case "$GIZOS_LUA_RUNTIME_SRC" in
  /*) runtime_src=$GIZOS_LUA_RUNTIME_SRC ;;
  *) runtime_src=$PWD/$GIZOS_LUA_RUNTIME_SRC ;;
esac
mkdir -p "$package_dir/.dart_tool"
printf '%s\n' "$runtime_src" > "$package_dir/.dart_tool/gizos-lua-runtime-src"
exec flutter "$@"
