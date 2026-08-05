#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-firmware-e2e.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"

cleanup() {
  if [[ -f "$docker_env_path" ]]; then
    bash "$setup_dir/docker-compose-down.sh" || true
  fi
  rm -f "$docker_env_path"
}
trap cleanup EXIT

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

cd "$repo_root"
git submodule update --init --recursive -- third_party/nanopb/upstream
mkdir -p "$script_dir/testdata/bin"
go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw
bash "$setup_dir/docker-compose-up.sh" --firmware-only
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

go test -p 1 -v -tags=gizclaw_e2e -count=1 -timeout=20m \
  ./tests/gizclaw-e2e/go/admin \
  -run '^(TestAdminAPIFirmwaresListGetAndConfigurePackages|TestAdminAPIFirmwareResourceLifecycle)$'
go test -p 1 -v -tags=gizclaw_e2e -count=1 -timeout=20m \
  ./tests/gizclaw-e2e/go/rpc \
  -run '^TestRegistrationBindsFirmwareRPC$'
go test -p 1 -v -tags=gizclaw_e2e -count=1 -timeout=20m \
  ./tests/gizclaw-e2e/cgo/rpc \
  -run '^(TestCSDKFirmwareRPC|TestCSDKFirmwareRPCMaximumName|TestCSDKFirmwareRequiresBinding)$'
go test -p 1 -v -tags=gizclaw_e2e -count=1 -timeout=20m \
  ./tests/gizclaw-e2e/cmd/admin \
  -run '^TestAdminFirmwaresUserStory$'
go test -p 1 -v -tags=gizclaw_e2e -count=1 -timeout=20m \
  ./tests/gizclaw-e2e/cmd/connect \
  -run '^TestRegistrationBindsFirmware$'
