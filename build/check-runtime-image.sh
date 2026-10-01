#!/usr/bin/env bash
# Exercise the shipped runtime, mounted SQLite/filesystem state and shutdown.
set -euo pipefail
[[ $# == 5 ]] || { echo "usage: $0 IMAGE VERSION SOURCE_COMMIT ARCH BINARY_SHA256" >&2; exit 2; }
image="$1" version="$2" source_commit="$3" arch="$4" binary_sha256="$5"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ "$arch" == amd64 || "$arch" == arm64 ]]
[[ "$source_commit" =~ ^[0-9a-f]{40}$ && "$binary_sha256" =~ ^[0-9a-f]{64}$ ]]
platform="linux/$arch"
docker image inspect "$image" | jq -e --arg arch "$arch" --arg version "$version" --arg commit "$source_commit" '
  .[0] | .Os == "linux" and .Architecture == $arch and
  .Config.User == "10001:10001" and .Config.Entrypoint == ["/usr/bin/gizclaw"] and
  .Config.Labels["org.opencontainers.image.version"] == $version and
  .Config.Labels["org.opencontainers.image.revision"] == $commit
' >/dev/null
[[ "$(docker run --rm --platform "$platform" "$image" --version)" == "gizclaw version $version" ]]
docker run --rm --platform "$platform" "$image" --help >/dev/null
docker run --rm --platform "$platform" --entrypoint sh "$image" -ec '
  test "$(sha256sum /usr/bin/gizclaw | cut -d " " -f1)" = "$1"
  ldd /usr/bin/gizclaw > /tmp/ldd
  cat /tmp/ldd
  ! grep -q "not found" /tmp/ldd
  test -s /etc/ssl/certs/ca-certificates.crt
  test "$(id -u)" = 10001
' sh "$binary_sha256"

work="$(mktemp -d)"
container=
volume="gizclaw-runtime-check-${arch}-$(date +%s)-$RANDOM"
cleanup() {
  [[ -z "$container" ]] || docker rm -f "$container" >/dev/null 2>&1 || true
  docker volume rm "$volume" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
docker volume create "$volume" >/dev/null
# Reuse the documented full configuration and its actual production Store wiring.
# SQLite makes this packaging gate independent of remote databases and providers.
# shellcheck disable=SC2016 # Literal placeholders belong to the mounted YAML.
sed -e '/^identity:/,+1d' -e '/^admin-public-key:/d' \
  -e 's/kind: postgresql/kind: sqlite/' -e 's|dsn: ${GIZCLAW_POSTGRES_DSN}|dir: data/database|' \
  -e '/^  sfu:/,/^  agent_host:/{ /^  agent_host:/!d; }' \
  "$repo_root/guides/snippets/server-storage-stores-services.yaml" >"$work/config.yaml"
# Config is read-only; generate identity outside it and supply it through env.
key="$(docker run --rm --platform "$platform" "$image" gen-key)"
[[ -n "$key" ]]
# shellcheck disable=SC2016
{ printf 'identity:\n  private-key: ${GIZCLAW_SERVER_PRIVATE_KEY}\n'; cat "$work/config.yaml"; } >"$work/mounted.yaml"

start() {
  container="$(docker run -d --platform "$platform" \
    --env GIZCLAW_SERVER_PRIVATE_KEY="$key" --mount "type=volume,src=$volume,dst=/var/lib/gizclaw" \
    --mount "type=bind,src=$work/mounted.yaml,dst=/var/lib/gizclaw/config.yaml,readonly" \
    --health-cmd 'curl -fsS --max-time 2 http://127.0.0.1:9820/server-info >/dev/null' \
    --health-interval 1s --health-timeout 3s --health-retries 60 "$image")"
  for ((attempt=0; attempt<90; attempt++)); do
    state="$(docker inspect --format '{{.State.Status}} {{.State.Health.Status}}' "$container")"
    [[ "$state" != 'running healthy' ]] || return 0
    [[ "$state" != exited* ]] || break
    sleep 1
  done
  docker logs "$container" >&2
  echo "Server did not become healthy: $state" >&2
  return 1
}
stop() {
  docker stop --time 30 "$container" >/dev/null
  [[ "$(docker inspect --format '{{.State.ExitCode}}' "$container")" == 0 ]]
  docker run --rm --platform "$platform" --mount "type=volume,src=$volume,dst=/var/lib/gizclaw" \
    --entrypoint sh "$image" -ec 'test ! -e serve.pid; test -d data/database; test -d data/files'
  docker rm "$container" >/dev/null
  container=
}
start
info="$(docker exec "$container" curl -fsS http://127.0.0.1:9820/server-info)"
jq -e --arg version "$version" --arg commit "$source_commit" '.version == $version and .build_commit == $commit' <<<"$info" >/dev/null
public_key="$(jq -er .public_key <<<"$info")"
docker exec "$container" sh -ec 'test -f serve.pid; printf persistence > data/runtime-image-check'
stop
start
docker exec "$container" sh -ec 'test "$(cat data/runtime-image-check)" = persistence'
info="$(docker exec "$container" curl -fsS http://127.0.0.1:9820/server-info)"
jq -e --arg key "$public_key" '.public_key == $key' <<<"$info" >/dev/null
stop
printf 'validated runtime linux/%s: CLI, libraries, mounted config/data, identity, health, restart, SIGTERM\n' "$arch"
