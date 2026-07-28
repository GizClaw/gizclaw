#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

cleanup() {
  bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1091
source "$script_dir/testdata/docker/current.env"
set +a
(cd "$repo_root" && go test -count=1 -v -tags gizclaw_e2e \
  -run '^TestGatewayOneEdgeFailureIsIsolated$' ./tests/gizclaw-e2e/go/edge)
