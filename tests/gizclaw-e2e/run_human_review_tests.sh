#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi

cleanup() {
  "$setup_dir/stop.sh" all >/dev/null 2>&1 || true
}
trap cleanup EXIT

run_pkg() {
  local pkg="$1"
  local run_regexp="$2"
  echo "==> go test $pkg -run $run_regexp"
  (cd "$repo_root" && go test -tags gizclaw_e2e -count=1 -run "$run_regexp" "$pkg")
}

echo "==> build e2e CLI"
"$setup_dir/build.sh" >/dev/null

echo "==> reset e2e data"
"$setup_dir/reset_data.sh" reset

run_pkg "./tests/gizclaw-e2e/go/chat" '^TestHumanReview$'
run_pkg "./tests/gizclaw-e2e/go/social" '^TestServerSocialRPCHumanReview$'

echo "==> human-review e2e run completed"
