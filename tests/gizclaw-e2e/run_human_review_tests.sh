#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"

cleanup() {
  bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
}
trap cleanup EXIT

run_pkg() {
  local pkg="$1"
  local run_regexp="$2"
  echo "==> go test $pkg -run $run_regexp"
  (cd "$repo_root" && go test -tags gizclaw_e2e -count=1 -run "$run_regexp" "$pkg")
}

echo "==> build host e2e CLI"
mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)

echo "==> start Docker e2e stack"
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$script_dir/testdata/docker/current.env"
set +a

run_pkg "./tests/gizclaw-e2e/go/chat" '^TestHumanReview$'
run_pkg "./tests/gizclaw-e2e/go/social" '^TestServerSocialRPCHumanReview$'

echo "==> human-review e2e run completed"
