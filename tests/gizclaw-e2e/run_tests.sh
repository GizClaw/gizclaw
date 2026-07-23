#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
default_skip_regexp='^(TestHumanReview|TestServerSocialRPCHumanReview)$'
go_test_timeout="45m"
full_deadline_seconds="${GIZCLAW_E2E_FULL_DEADLINE_SECONDS:-5400}"
gate_started=$SECONDS
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-e2e-run.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
full_watchdog_pid=""
active_command_pid=""
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
export GIZCLAW_E2E_REQUIRE_LIVE=1

cleanup() {
  if [[ -n "$active_command_pid" ]]; then
    terminate_process_tree "$active_command_pid" TERM
    active_command_pid=""
  fi
  stop_full_watchdog
  if [[ -f "$docker_env_path" ]]; then
    run_timed "docker:cleanup" bash "$setup_dir/docker-compose-down.sh" || true
  fi
  rm -f "$docker_env_path"
  echo "==> e2e cleanup done total_elapsed_seconds=$((SECONDS - gate_started))"
}
trap cleanup EXIT

require_positive_seconds() {
	local name="$1"
	local value="$2"
	if [[ ! "$value" =~ ^[1-9][0-9]*$ ]]; then
		echo "$name must be a positive integer number of seconds, got: $value" >&2
		exit 2
	fi
}

terminate_process_tree() {
	local pid="$1"
	local signal="$2"
	local child
	while IFS= read -r child; do
		if [[ -n "$child" ]]; then
			terminate_process_tree "$child" "$signal"
		fi
	done < <(pgrep -P "$pid" 2>/dev/null || true)
	kill "-$signal" "$pid" >/dev/null 2>&1 || true
}

terminate_process_tree_gracefully() {
	local root_pid="$1"
	local process_pids
	process_pids="$(process_tree_pids "$root_pid")"
	local pid
	for pid in $process_pids; do
		kill -TERM "$pid" >/dev/null 2>&1 || true
	done
	sleep 1
	for pid in $process_pids; do
		kill -KILL "$pid" >/dev/null 2>&1 || true
	done
}

process_tree_pids() {
	local pid="$1"
	local child
	while IFS= read -r child; do
		if [[ -n "$child" ]]; then
			process_tree_pids "$child"
		fi
	done < <(pgrep -P "$pid" 2>/dev/null || true)
	echo "$pid"
}

phase_deadline_seconds() {
	local phase="$1"
	case "$phase" in
		preflight:*) echo "${GIZCLAW_E2E_PREFLIGHT_DEADLINE_SECONDS:-900}" ;;
		docker:setup) echo "${GIZCLAW_E2E_DOCKER_SETUP_DEADLINE_SECONDS:-1800}" ;;
		docker:cleanup) echo "${GIZCLAW_E2E_DOCKER_CLEANUP_DEADLINE_SECONDS:-300}" ;;
		go:chat | chat:*) echo "${GIZCLAW_E2E_CHAT_DEADLINE_SECONDS:-2700}" ;;
		cli) echo "${GIZCLAW_E2E_CLI_DEADLINE_SECONDS:-1800}" ;;
		*) echo "${GIZCLAW_E2E_PHASE_DEADLINE_SECONDS:-900}" ;;
	esac
}

start_full_watchdog() {
	local runner_pid="$$"
	(
		sleep "$full_deadline_seconds"
		echo "full e2e deadline exceeded after ${full_deadline_seconds}s" >&2
		kill -TERM "$runner_pid" >/dev/null 2>&1 || true
	) &
	full_watchdog_pid="$!"
}

stop_full_watchdog() {
	if [[ -z "$full_watchdog_pid" ]]; then
		return
	fi
	kill "$full_watchdog_pid" >/dev/null 2>&1 || true
	wait "$full_watchdog_pid" >/dev/null 2>&1 || true
	full_watchdog_pid=""
}

validate_deadlines() {
	require_positive_seconds GIZCLAW_E2E_FULL_DEADLINE_SECONDS "$full_deadline_seconds"
	local phase deadline
	for phase in preflight:validate docker:setup docker:cleanup go:chat cli go:validate; do
		deadline="$(phase_deadline_seconds "$phase")"
		require_positive_seconds "deadline for $phase" "$deadline"
	done
}

deadline_exit() {
	echo "full e2e gate terminated by deadline or signal" >&2
	if [[ -n "$active_command_pid" ]]; then
		terminate_process_tree_gracefully "$active_command_pid"
		active_command_pid=""
	fi
	exit 124
}
trap deadline_exit INT TERM

run_timed() {
	local phase="$1"
	shift
	local deadline
	deadline="$(phase_deadline_seconds "$phase")"
	require_positive_seconds "deadline for $phase" "$deadline"
	local started=$SECONDS
	local status=0
	local marker
	marker="$(mktemp "${TMPDIR:-/tmp}/gizclaw-e2e-deadline.XXXXXX")"
	rm -f "$marker"
	echo "==> phase start: $phase deadline_seconds=$deadline"
	"$@" &
	local command_pid="$!"
	active_command_pid="$command_pid"
	(
		sleep "$deadline"
		if kill -0 "$command_pid" >/dev/null 2>&1; then
			: >"$marker"
			echo "phase deadline exceeded: $phase after ${deadline}s" >&2
			terminate_process_tree_gracefully "$command_pid"
		fi
	) &
	local watchdog_pid="$!"
	wait "$command_pid" || status=$?
	active_command_pid=""
	kill "$watchdog_pid" >/dev/null 2>&1 || true
	wait "$watchdog_pid" >/dev/null 2>&1 || true
	if [[ -f "$marker" ]]; then
		status=124
	fi
	rm -f "$marker"
	echo "==> phase done: $phase status=$status elapsed_seconds=$((SECONDS - started))"
	return "$status"
}

prepare_node_dependencies() {
	(cd "$repo_root" && npm ci)
}

prepare_nanopb() {
	(cd "$repo_root" && git submodule update --init --recursive -- third_party/nanopb/upstream)
}

build_host_cli() {
	mkdir -p "$script_dir/testdata/bin"
	(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
}

start_docker_stack() {
	if [[ ! -f "$env_file" ]]; then
		echo "missing $env_file; copy .env.example and fill provider credentials before Docker e2e" >&2
		exit 2
	fi
	bash "$setup_dir/docker-compose-up.sh"
}

run_pkg() {
  local pkg="$1"
  echo "==> go test $pkg"
  (cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -skip "$default_skip_regexp" "$pkg")
}

run_pkg_serial() {
	local pkg="$1"
	echo "==> go test -p 1 $pkg"
	(cd "$repo_root" && go test -p 1 -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -skip "$default_skip_regexp" "$pkg")
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
	local status=0
	chat_skip_regexp="^($(IFS='|'; echo "${chat_live_tests[*]}")|TestHumanReview|TestServerSocialRPCHumanReview)$"

  echo "==> go test $chat_pkg unit"
  (cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" -skip "$chat_skip_regexp" "$chat_pkg") || status=$?

	local test_regex
	for test_regex in "${chat_default_live_patterns[@]}"; do
		run_timed "chat:$test_regex" run_pkg_test_regex "$chat_pkg" "$test_regex" || status=$?
	done
	return "$status"
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

validate_deadlines
start_full_watchdog

run_timed "preflight:npm-ci" prepare_node_dependencies
run_timed "preflight:nanopb" prepare_nanopb

run_timed "preflight:host-cli" build_host_cli

run_timed "docker:setup" start_docker_stack
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

run_timed "javascript" run_js_rpc_tests
run_timed "desktop" run_desktop_tests
run_timed "cgo:rpc" run_pkg "./tests/gizclaw-e2e/cgo/rpc"
run_timed "cgo:telemetry" run_pkg "./tests/gizclaw-e2e/cgo/telemetry"
run_timed "cgo:chat" run_pkg "./tests/gizclaw-e2e/cgo/chat"
run_timed "cgo:social" run_pkg "./tests/gizclaw-e2e/cgo/social"
run_timed "go:admin" run_pkg "./tests/gizclaw-e2e/go/admin"
run_timed "go:chat" run_chat_pkg
run_timed "go:gameplay" run_pkg "./tests/gizclaw-e2e/go/gameplay"
run_timed "go:rpc" run_pkg "./tests/gizclaw-e2e/go/rpc"
run_timed "go:social" run_pkg "./tests/gizclaw-e2e/go/social"
run_timed "cli" run_pkg_serial "./tests/gizclaw-e2e/cmd/..."

echo "==> e2e run completed"
