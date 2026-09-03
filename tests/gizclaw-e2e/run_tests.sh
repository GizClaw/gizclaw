#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"
default_skip_regexp='^(TestHumanReview|TestServerSocialRPCHumanReview|TestAdminLogStreamVolcSmoke)$'
go_test_timeout="45m"
full_deadline_seconds="${GIZCLAW_E2E_FULL_DEADLINE_SECONDS:-5400}"
gate_started=$SECONDS
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-e2e-run.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
full_watchdog_pid=""
active_command_pid=""
active_phase=""
failure_diagnostics_collected=0
stack_started=0

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

cleanup() {
  if [[ -n "$active_command_pid" ]]; then
    terminate_process_tree "$active_command_pid" TERM
    active_command_pid=""
  fi
  stop_full_watchdog
	if [[ "$stack_started" == "1" && -f "$docker_env_path" ]]; then
		run_timed "docker:cleanup" bash "$setup_dir/docker-compose-down.sh" || true
		stack_started=0
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
		giztest:*) echo "${GIZCLAW_E2E_CHAT_DEADLINE_SECONDS:-2700}" ;;
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
	for phase in preflight:validate docker:setup docker:cleanup giztest:standard giztest:c-sdk cli go:validate; do
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
	collect_failure_diagnostics
	exit 124
}
trap deadline_exit INT TERM

docker_compose_args() {
	local compose_file="${GIZCLAW_E2E_DOCKER_COMPOSE_FILE:-}"
	if [[ -z "$compose_file" ]]; then
		return 1
	fi
	printf '%s\n' -f "$compose_file"
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		printf '%s\n' -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY"
	fi
}

diagnose_server_info() {
	local label="$1"
	local endpoint="$2"
	if [[ -z "$endpoint" ]]; then
		return
	fi
	local url="$endpoint"
	if [[ "$url" != http://* && "$url" != https://* ]]; then
		url="http://$url"
	fi
	local result
	result="$(curl --silent --show-error --output /dev/null \
		--connect-timeout 2 --max-time 3 \
		--write-out 'http_code=%{http_code} remote_ip=%{remote_ip} connect_seconds=%{time_connect} total_seconds=%{time_total}' \
		"${url%/}/server-info" 2>&1)" || true
	echo "diagnostic server-info $label: $result" >&2
}

redact_failure_diagnostics() {
	python3 "$setup_dir/redact_diagnostics.py"
}

tail_service_log() {
	local project="$1"
	local service="$2"
	local log_path="$3"
	shift 3
	echo "==> diagnostic app log service=$service tail=200" >&2
	docker compose -p "$project" "$@" exec -T "$service" \
		sh -c 'if test -f "$1"; then tail -n 200 "$1"; else echo "log file unavailable: $1"; fi' \
		sh "$log_path" 2>&1 | redact_failure_diagnostics >&2 || true
}

collect_failure_diagnostics() {
	if [[ "$failure_diagnostics_collected" == "1" ]]; then
		return
	fi
	if [[ ! -f "$docker_env_path" ]]; then
		return
	fi
	failure_diagnostics_collected=1
	set -a
	# shellcheck disable=SC1090
	source "$docker_env_path"
	set +a
	local project="${GIZCLAW_E2E_DOCKER_PROJECT:-}"
	if [[ -z "$project" ]]; then
		return
	fi
	local -a compose_args=()
	while IFS= read -r arg; do
		compose_args+=("$arg")
	done < <(docker_compose_args || true)
	if ((${#compose_args[@]} == 0)); then
		return
	fi

	echo "==> e2e failure diagnostics phase=${active_phase:-unknown} project=$project" >&2
	docker compose -p "$project" "${compose_args[@]}" ps --all >&2 2>&1 || true
	local container_ids
	container_ids="$(docker compose -p "$project" "${compose_args[@]}" ps --all -q 2>/dev/null || true)"
	if [[ -n "$container_ids" ]]; then
		# shellcheck disable=SC2086 # Docker expects one argument per container ID.
		docker stats --no-stream --format \
			'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.PIDs}}\t{{.NetIO}}' \
			$container_ids >&2 2>&1 || true
	fi
	diagnose_server_info server "${GIZCLAW_E2E_SERVER_ENDPOINT:-}"
	diagnose_server_info edge "${GIZCLAW_E2E_EDGE_ENDPOINT:-}"
	diagnose_server_info edge2 "${GIZCLAW_E2E_EDGE2_ENDPOINT:-}"

	tail_service_log "$project" server \
		/src/tests/gizclaw-e2e/testdata/server-workspace/gizclaw-server.log \
		"${compose_args[@]}"
	tail_service_log "$project" edge \
		/src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log \
		"${compose_args[@]}"
	tail_service_log "$project" edge2 \
		/src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log \
		"${compose_args[@]}"
	for service in turn server edge edge2; do
		echo "==> diagnostic container log service=$service tail=80" >&2
		docker compose -p "$project" "${compose_args[@]}" logs \
			--no-color --tail=80 "$service" 2>&1 | redact_failure_diagnostics >&2 || true
	done
}

run_timed() {
	local phase="$1"
	shift
	active_phase="$phase"
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
	if [[ "$status" != "0" ]]; then
		collect_failure_diagnostics
	fi
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

# The cgo runner drives the same scenarios through sdk/c/gizclaw and
# sdk/c/gizclaw_control instead of the Go SDK, so it needs nanopb in place.
build_c_giztest_runner() {
	mkdir -p "$script_dir/testdata/bin"
	(cd "$repo_root" && go build -o "$script_dir/testdata/bin/giztest-c" ./tests/gizclaw-e2e/cgo/giztest)
}

start_docker_stack() {
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

run_js_rpc_tests() {
	echo "==> npm test --workspace @gizclaw/gizclaw"
	(cd "$repo_root" && npm test --workspace @gizclaw/gizclaw)

	echo "==> node tests/gizclaw-e2e/js/admin"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:admin)

	echo "==> node tests/gizclaw-e2e/js/admin telemetry"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:admin-telemetry)

	echo "==> node tests/gizclaw-e2e/js/rpc"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:rpc)

	echo "==> node tests/gizclaw-e2e/js/streams"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:streams)
}

# The JavaScript and Flutter runners execute the same scenario documents as the
# Go runner, through their own SDKs. Each reports documents that use step kinds
# it does not implement as skipped.
run_js_giztest() {
	local report="$script_dir/testdata/giztest-js-report.json"
	echo "==> node tests/gizclaw-e2e/js/giztest"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:giztest-unit)
	(cd "$repo_root/tests/gizclaw-e2e/js" &&
		npm run giztest -- run "$script_dir/giztest" --parallel 4 --output "$report")
}

run_flutter_giztest() {
	local package_dir="$script_dir/flutter/giztest"
	local report="$script_dir/testdata/giztest-flutter-report.json"
	local binary
	if ! command -v flutter >/dev/null 2>&1; then
		echo "==> skipping tests/gizclaw-e2e/flutter/giztest: flutter is not installed" >&2
		return 0
	fi
	case "$(uname -s)" in
		Darwin) binary="$package_dir/build/macos/Build/Products/Release/giztest.app/Contents/MacOS/giztest" ;;
		Linux) binary="$package_dir/build/linux/x64/release/bundle/giztest" ;;
		*)
			echo "==> skipping tests/gizclaw-e2e/flutter/giztest: unsupported host" >&2
			return 0
			;;
	esac
	echo "==> flutter test $package_dir"
	(cd "$package_dir" && flutter pub get && flutter test)
	echo "==> flutter build $package_dir"
	case "$(uname -s)" in
		Darwin) (cd "$package_dir" && flutter build macos --release) ;;
		Linux) (cd "$package_dir" && flutter build linux --release) ;;
	esac
	echo "==> $binary run"
	"$binary" run "$script_dir/giztest" --parallel 4 --output "$report"
}

run_standard_giztest() {
	local giztest_dir="$script_dir/giztest"
	local report="$script_dir/testdata/giztest-standard-report.json"
	local -a files=()
	while IFS= read -r file; do files+=("$file"); done < <(
		find "$giztest_dir" -maxdepth 1 -type f -name '*.giztest.yaml' \
			! -name 'benchmark.*' ! -name 'review.*' ! -name 'failure-cleanup.giztest.yaml' -print | sort
	)
	files+=(
		"$giztest_dir/benchmark.doubao-realtime-conversation.concurrency-1.giztest.yaml"
		"$giztest_dir/benchmark.doubao-realtime-conversation.realtime-concurrency-1.giztest.yaml"
		"$giztest_dir/benchmark.doubao-realtime-conversation.concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.doubao-realtime-conversation.concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.flowcraft-voice-assistant.concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.flowcraft-voice-assistant.concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.eino-concurrency-assistant.concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.eino-concurrency-assistant.concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.volc-ast-translate.concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.volc-ast-translate.concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.doubao-realtime-conversation.realtime-concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.doubao-realtime-conversation.realtime-concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.flowcraft-voice-assistant.realtime-concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.flowcraft-voice-assistant.realtime-concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.eino-concurrency-assistant.realtime-concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.eino-concurrency-assistant.realtime-concurrency-interrupt-10.giztest.yaml"
		"$giztest_dir/benchmark.volc-ast-translate.realtime-concurrency-10.giztest.yaml"
		"$giztest_dir/benchmark.volc-ast-translate.realtime-concurrency-interrupt-10.giztest.yaml"
	)
	(cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run "${files[@]}" --parallel 10 --output "$report")
}

# The C runner supports rpc, client_rpc, and the /gizclaw/v1 http surface. It
# runs the scenarios built from those operations; validate rejects anything
# else, so an unsupported step can never pass silently here.
run_c_giztest() {
	local giztest_dir="$script_dir/giztest"
	local report="$script_dir/testdata/giztest-c-report.json"
	local -a files=()
	while IFS= read -r file; do files+=("$file"); done < <(
		find "$giztest_dir" -maxdepth 1 -type f \
			\( -name 'all.ping.giztest.yaml' -o -name 'server.api_key.*.giztest.yaml' \
			-o -name 'server.contacts.*.giztest.yaml' -o -name 'server.device.*.giztest.yaml' \) \
			-print | sort
	)
	local -a validate_args=()
	local file
	for file in "${files[@]}"; do validate_args+=(--file "$file"); done
	(cd "$repo_root" && "$script_dir/testdata/bin/giztest-c" test validate "${validate_args[@]}")
	(cd "$repo_root" && "$script_dir/testdata/bin/giztest-c" test run "${files[@]}" --parallel 4 --output "$report")
}

run_failure_cleanup_giztest() {
	local report="$script_dir/testdata/giztest-failure-cleanup-report.json"
	if (cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run \
		"$script_dir/giztest/all.ping.giztest.yaml" \
		"$script_dir/giztest/failure-cleanup.giztest.yaml" \
		--parallel 2 --output "$report"); then
		echo "failure-cleanup Giztest unexpectedly passed" >&2
		return 1
	fi
	python3 - "$report" <<'PY'
import json, sys
report = json.load(open(sys.argv[1], encoding="utf-8"))
tasks = {task["name"]: task for task in report["tasks"]}
assert tasks["all.ping"]["status"] == "passed"
failed = tasks["failure-cleanup"]
assert failed["status"] == "failed" and failed.get("cleanup")
assert all(step["status"] == "passed" for step in failed["cleanup"])
PY
}

validate_deadlines
start_full_watchdog

run_timed "preflight:diagnostic-redaction" python3 "$setup_dir/redact_diagnostics_test.py"
run_timed "preflight:npm-ci" prepare_node_dependencies
run_timed "preflight:nanopb" prepare_nanopb

run_timed "preflight:host-cli" build_host_cli
run_timed "preflight:c-giztest-runner" build_c_giztest_runner

stack_started=1
run_timed "docker:setup" start_docker_stack
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

run_timed "javascript" run_js_rpc_tests
run_timed "giztest:javascript" run_js_giztest
run_timed "giztest:flutter" run_flutter_giztest
run_timed "cgo:rpc" run_pkg "./tests/gizclaw-e2e/cgo/rpc"
run_timed "cgo:telemetry" run_pkg "./tests/gizclaw-e2e/cgo/telemetry"
run_timed "cgo:chat" run_pkg "./tests/gizclaw-e2e/cgo/chat"
run_timed "cgo:media" run_pkg "./tests/gizclaw-e2e/cgo/media"
run_timed "cgo:social" run_pkg "./tests/gizclaw-e2e/cgo/social"
run_timed "go:admin" run_pkg "./tests/gizclaw-e2e/go/admin"
run_timed "go:openai" run_pkg "./tests/gizclaw-e2e/go/openai"
run_timed "cli" run_pkg_serial "./tests/gizclaw-e2e/cmd/..."
run_timed "giztest:standard" run_standard_giztest
run_timed "giztest:failure-cleanup" run_failure_cleanup_giztest
run_timed "giztest:c-sdk" run_c_giztest

run_timed "docker:standard-cleanup" bash "$setup_dir/docker-compose-down.sh"
stack_started=0

echo "==> e2e run completed"
