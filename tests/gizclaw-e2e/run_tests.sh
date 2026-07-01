#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
selected_config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"
default_skip_regexp='^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$'
go_test_timeout="45m"
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

if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$env_file"
  set +a
fi
if [[ -n "$selected_config_home" ]]; then
  export GIZCLAW_E2E_CONFIG_HOME="$selected_config_home"
fi

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

cleanup() {
  "$setup_dir/stop.sh" all >/dev/null 2>&1 || true
}
trap cleanup EXIT

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

echo "==> build e2e CLI"
"$setup_dir/build.sh" >/dev/null

echo "==> reset e2e data"
"$setup_dir/reset_data.sh" reset

run_pkg "./tests/gizclaw-e2e/go/admin"
run_chat_pkg
run_pkg "./tests/gizclaw-e2e/go/rpc"
run_pkg "./tests/gizclaw-e2e/go/social"
run_pkg "./tests/gizclaw-e2e/cmd/connect"

echo "==> e2e run completed"
