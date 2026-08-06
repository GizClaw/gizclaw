#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
e2e_dir="$(cd "$script_dir/.." && pwd)"
repo_root="$(cd "$e2e_dir/../.." && pwd)"
output_dir="$e2e_dir/testdata/bin"
output_path="$output_dir/gizclaw-linux"
temporary_output="$output_path.tmp.$$"
build_pid=""
builder_suffix="$(printf '%s-%s' "${USER:-user}" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
builder_container="gizclaw-linux-cgo-$builder_suffix"

cleanup_build() {
  if [[ -n "$build_pid" ]] && kill -0 "$build_pid" 2>/dev/null; then
    kill -TERM "$build_pid" 2>/dev/null || true
    wait "$build_pid" 2>/dev/null || true
  fi
  docker rm -f "$builder_container" >/dev/null 2>&1 || true
  rm -f "$temporary_output"
}
trap cleanup_build EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

docker_arch="$(docker info --format '{{.Architecture}}')"
case "$docker_arch" in
  aarch64 | arm64)
    docker_platform="linux/arm64"
    ;;
  x86_64 | amd64)
    docker_platform="linux/amd64"
    ;;
  *)
    echo "unsupported Docker architecture for Linux CGO build: $docker_arch" >&2
    exit 2
    ;;
esac

platform_slug="${docker_platform//\//-}"
base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:${platform_slug}-cn-base}"
if ! docker image inspect "$base_image" >/dev/null 2>&1; then
  echo "==> build missing e2e base image: image=$base_image platform=$docker_platform"
  docker build \
    --platform="$docker_platform" \
    -f "$repo_root/build/Dockerfile.cn.base" \
    -t "$base_image" \
    "$repo_root/build"
fi

module_cache="$(go env GOMODCACHE)"
mkdir -p "$output_dir"
rm -f "$temporary_output"

echo "==> Linux CGO build started: platform=$docker_platform image=$base_image output=$output_path"
docker run --rm \
  --name "$builder_container" \
  --platform "$docker_platform" \
  --user "$(id -u):$(id -g)" \
  --env HOME=/tmp \
  --env GOCACHE=/tmp/gizclaw-go-build-cache \
  --env GOMODCACHE=/go/pkg/mod \
  --volume "$repo_root:/src" \
  --volume "$module_cache:/go/pkg/mod" \
  --workdir /src \
  --entrypoint bash \
  "$base_image" \
  -lc "CGO_ENABLED=1 GOOS=linux go build -trimpath -o '/src/tests/gizclaw-e2e/testdata/bin/$(basename "$temporary_output")' ./cmd/gizclaw" &
build_pid="$!"
build_started="$SECONDS"
last_heartbeat=0
while kill -0 "$build_pid" 2>/dev/null; do
  sleep 1
  elapsed=$((SECONDS - build_started))
  if ((elapsed >= last_heartbeat + 10)); then
    echo "==> Linux CGO build heartbeat: status=running elapsed_seconds=$elapsed"
    last_heartbeat="$elapsed"
  fi
done
build_status=0
wait "$build_pid" || build_status="$?"
build_pid=""
if ((build_status != 0)); then
  echo "Linux CGO build failed: exit_code=$build_status" >&2
  exit "$build_status"
fi
mv "$temporary_output" "$output_path"

docker run --rm \
  --platform "$docker_platform" \
  --volume "$output_path:/usr/local/bin/gizclaw:ro" \
  --entrypoint bash \
  "$base_image" \
  -lc 'set -e; test -x /usr/local/bin/gizclaw; ldd /usr/local/bin/gizclaw; /usr/local/bin/gizclaw --help >/dev/null'
echo "==> Linux CGO build passed: platform=$docker_platform bytes=$(wc -c < "$output_path" | tr -d ' ') output=$output_path"
