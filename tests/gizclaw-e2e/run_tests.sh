#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
default_skip_regexp='^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$'
go_test_timeout="45m"
docker_project="${GIZCLAW_E2E_DOCKER_PROJECT:-}"
docker_started=0
chat_pkg="./tests/gizclaw-e2e/go/chat"
chat_live_tests=(
  TestPushToTalkRoundtrip
  TestHistoryReplay
  TestRealtimeRoundtrip
  TestRealtimeInterrupt
  TestRealtimeAutoSplitHistory
  TestPushToTalkInterrupt
)
chat_default_live_patterns=(
  '^TestPushToTalkRoundtrip$'
  '^TestRealtimeRoundtrip$'
  '^TestHistoryReplay$'
  '^TestRealtimeInterrupt$'
  '^TestRealtimeAutoSplitHistory$'
  '^TestPushToTalkInterrupt$'
)

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

cleanup() {
  if [[ "$docker_started" == "1" ]]; then
    bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

start_docker_stack() {
	if [[ ! -f "$env_file" ]]; then
		echo "missing $env_file; copy .env.example and fill provider credentials before Docker e2e" >&2
		exit 2
	fi
	docker_started=1
	bash "$setup_dir/docker-compose-up.sh"
	set -a
	# shellcheck disable=SC1090
	source "$script_dir/testdata/docker/current.env"
	set +a
	docker_project="$GIZCLAW_E2E_DOCKER_PROJECT"
}

run_pkg() {
  local pkg="$1"
  echo "==> go test $pkg"
  (cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -skip "$default_skip_regexp" "$pkg")
}

run_pkg_test() {
	local pkg="$1"
	local test_name="$2"
	echo "==> go test $pkg -run ^${test_name}$"
	(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -run "^${test_name}$" -skip "$default_skip_regexp" "$pkg")
}

run_pkg_test_regex() {
	local pkg="$1"
	local test_regex="$2"
	echo "==> go test $pkg -run ${test_regex}"
	(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -run "$test_regex" -skip "$default_skip_regexp" "$pkg")
}

run_chat_pkg() {
	local chat_skip_regexp
	chat_skip_regexp="^($(IFS='|'; echo "${chat_live_tests[*]}")|TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$"

  echo "==> go test $chat_pkg unit"
  (cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -skip "$chat_skip_regexp" "$chat_pkg")

	local test_regex
	for test_regex in "${chat_default_live_patterns[@]}"; do
		run_pkg_test_regex "$chat_pkg" "$test_regex"
	done
}

run_js_rpc_tests() {
	echo "==> npm test --workspace @gizclaw/gizclaw"
	(cd "$repo_root" && npm test --workspace @gizclaw/gizclaw)

	echo "==> node tests/gizclaw-e2e/js/admin"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:admin)

	echo "==> node tests/gizclaw-e2e/js/admin telemetry"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:admin-telemetry)

	echo "==> node tests/gizclaw-e2e/js/rpc"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:rpc)
}

run_desktop_tests() {
	echo "==> go test tests/gizclaw-e2e/desktop"
	(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" ./tests/gizclaw-e2e/desktop/...)
}

echo "==> build host e2e CLI"
mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)

start_docker_stack

run_js_rpc_tests
run_desktop_tests
run_pkg "./tests/gizclaw-e2e/cgo/rpc"
run_pkg "./tests/gizclaw-e2e/cgo/chat"
run_pkg "./tests/gizclaw-e2e/cgo/social"
run_pkg "./tests/gizclaw-e2e/go/admin"
run_chat_pkg
run_pkg "./tests/gizclaw-e2e/go/gameplay"
run_pkg "./tests/gizclaw-e2e/go/rpc"
run_pkg "./tests/gizclaw-e2e/go/social"
run_pkg "./tests/gizclaw-e2e/cmd/connect"

echo "==> e2e run completed"
