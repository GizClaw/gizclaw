#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-e2e-sibling-close.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

cleanup() {
  if [[ -f "$docker_env_path" ]]; then
    bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
  fi
  rm -f "$docker_env_path"
}
trap cleanup EXIT

echo "==> install locked Node workspace"
(cd "$repo_root" && npm ci)

echo "==> initialize nanopb"
(cd "$repo_root" && git submodule update --init third_party/nanopb/upstream)

echo "==> build host e2e CLI"
mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)

echo "==> start Docker e2e stack"
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

for attempt in 1 2 3; do
  echo "==> JavaScript sibling-close attempt $attempt/3"
  (cd "$repo_root" && npm --prefix tests/gizclaw-e2e/js run test:streams)
done

echo "==> C/cgo sibling-close attempts 1-3/3"
(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=3 -timeout 45m \
  -run '^TestCSDKConcurrentServiceStreams$' \
  ./tests/gizclaw-e2e/cgo/rpc)

echo "==> Go sibling-close attempts 1-3/3"
(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=3 -timeout 45m \
  -run '^TestConcurrentServiceStreams$' \
  ./tests/gizclaw-e2e/go/rpc)

echo "==> sibling-close e2e run completed"
