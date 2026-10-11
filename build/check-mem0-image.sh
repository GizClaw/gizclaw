#!/usr/bin/env bash
set -euo pipefail
[[ $# == 4 ]] || { echo "usage: $0 IMAGE VERSION SOURCE_COMMIT ARCH" >&2; exit 2; }
image="$1" version="$2" commit="$3" arch="$4"
[[ "$arch" == amd64 || "$arch" == arm64 ]]
docker image inspect "$image" | jq -e --arg version "$version" --arg commit "$commit" --arg arch "$arch" '
  .[0] | .Os == "linux" and .Architecture == $arch and .Config.User == "10001:10001" and
  .Config.Entrypoint == ["python","-m","gizclaw_mem0"] and
  .Config.Labels["org.opencontainers.image.version"] == $version and
  .Config.Labels["org.opencontainers.image.revision"] == $commit
' >/dev/null
[[ "$(docker run --rm --platform "linux/$arch" "$image" --version)" == "$version" ]]
docker run --rm --platform "linux/$arch" "$image" --help >/dev/null
docker run --rm --platform "linux/$arch" --entrypoint python "$image" -c \
  'import os; from gizclaw_mem0 import server; assert os.getuid() == 10001; assert server.app is not None'
