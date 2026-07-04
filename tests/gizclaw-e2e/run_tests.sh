#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
docker_dir="$script_dir/docker"
env_file="$script_dir/.env"
selected_config_home="${GIZCLAW_E2E_CONFIG_HOME:-}"
default_skip_regexp='^(TestHumanReview|TestServerSocialRPCHumanReview|TestSocialRealtimeHistoryRPC)$'
go_test_timeout="45m"
use_docker="${GIZCLAW_E2E_USE_DOCKER:-1}"
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
  if [[ "$docker_started" == "1" ]]; then
    docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" down -v >/dev/null 2>&1 || true
  else
    "$setup_dir/stop.sh" all >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

wait_http_ready() {
	local url="$1"
	local label="$2"
	local service="${3:-}"
	for _ in {1..300}; do
		if curl -fsS --max-time 1 "$url" >/dev/null 2>&1; then
			return 0
		fi
		if [[ "$docker_started" == "1" && -n "$service" ]]; then
			local container_id container_state exit_code
			container_id="$(docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" ps -q "$service" 2>/dev/null || true)"
			if [[ -n "$container_id" ]]; then
				container_state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
				exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$container_id" 2>/dev/null || true)"
				if [[ "$container_state" == "exited" || "$container_state" == "dead" ]]; then
					echo "$label container exited before becoming ready at $url (state=$container_state exit=$exit_code)" >&2
					docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" logs --tail=200 "$service" >&2 || true
					return 1
				fi
			fi
		fi
		sleep 0.2
	done
	echo "$label did not become ready at $url" >&2
	if [[ "$docker_started" == "1" && -n "$service" ]]; then
		docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" logs --tail=200 "$service" >&2 || true
	fi
	return 1
}

wait_docker_ready_file() {
	local service="$1"
	local ready_file="$2"
	local label="$3"
	for _ in {1..300}; do
		local container_id container_state exit_code
		container_id="$(docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" ps -q "$service" 2>/dev/null || true)"
		if [[ -n "$container_id" ]]; then
			container_state="$(docker inspect --format '{{.State.Status}}' "$container_id" 2>/dev/null || true)"
			exit_code="$(docker inspect --format '{{.State.ExitCode}}' "$container_id" 2>/dev/null || true)"
			if [[ "$container_state" == "exited" || "$container_state" == "dead" ]]; then
				echo "$label container exited before ready marker $ready_file (state=$container_state exit=$exit_code)" >&2
				docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" logs --tail=200 "$service" >&2 || true
				return 1
			fi
			if docker exec "$container_id" test -f "$ready_file" >/dev/null 2>&1; then
				return 0
			fi
		fi
		sleep 0.2
	done
	echo "$label did not create ready marker $ready_file" >&2
	docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" logs --tail=200 "$service" >&2 || true
	return 1
}

pick_free_tcp_port() {
	local port
	for _ in {1..100}; do
		port=$((20000 + RANDOM % 30000))
		if ! (: >"/dev/tcp/127.0.0.1/$port") >/dev/null 2>&1; then
			echo "$port"
			return 0
		fi
	done
	echo "failed to find a free local TCP port" >&2
	return 1
}

rewrite_endpoint_configs() {
	local root="$1"
	local endpoint="$2"
	local file
	while IFS= read -r file; do
		perl -0pi -e 's/(endpoint:\s*)[^\s]+/${1}'"$endpoint"'/g' "$file"
	done < <(find "$root" -type f -name config.yaml -print)
}

materialize_docker_test_config() {
	local endpoint="$1"
	local desktop_url="$2"
	local state_dir="$script_dir/testdata/docker/$docker_project"
	local identities_home="$state_dir/identities"
	local config_home="$state_dir/cmd-config-home"

	rm -rf "$state_dir"
	mkdir -p "$state_dir"
	cp -R "$script_dir/testdata/identities" "$identities_home"
	cp -R "$script_dir/testdata/cmd-config-home" "$config_home"
	rewrite_endpoint_configs "$identities_home" "$endpoint"
	rewrite_endpoint_configs "$config_home" "$endpoint"

	export GIZCLAW_E2E_CONFIG_HOME="$config_home"
	export GIZCLAW_E2E_IDENTITIES_HOME="$identities_home"
	export GIZCLAW_E2E_JS_IDENTITY_DIR="$identities_home/peer"
	export GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR="$identities_home/admin"
	export GIZCLAW_E2E_SERVER_ENDPOINT="$endpoint"
	export GIZCLAW_E2E_DESKTOP_URL="$desktop_url"

	cat >"$state_dir/docker.env" <<EOF
GIZCLAW_E2E_CONFIG_HOME=$config_home
GIZCLAW_E2E_IDENTITIES_HOME=$identities_home
GIZCLAW_E2E_JS_IDENTITY_DIR=$identities_home/peer
GIZCLAW_E2E_JS_ADMIN_IDENTITY_DIR=$identities_home/admin
GIZCLAW_E2E_SERVER_ENDPOINT=$endpoint
GIZCLAW_E2E_DESKTOP_URL=$desktop_url
GIZCLAW_E2E_DOCKER_PROJECT=$docker_project
GIZCLAW_E2E_DOCKER_SERVER_PORT=${GIZCLAW_E2E_DOCKER_SERVER_PORT:-}
EOF
	echo "==> docker e2e env: $state_dir/docker.env"
}

validate_docker_project() {
	if [[ ! "$docker_project" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
		echo "invalid GIZCLAW_E2E_DOCKER_PROJECT: $docker_project" >&2
		echo "Docker Compose project names must start with a lowercase letter or digit and contain only lowercase letters, digits, underscores, or dashes." >&2
		exit 2
	fi
}

start_docker_stack() {
	if [[ ! -f "$env_file" ]]; then
		echo "missing $env_file; copy .env.example and fill provider credentials before Docker e2e" >&2
		exit 2
	fi
	if [[ -z "$docker_project" ]]; then
		local suffix
		suffix="$(printf '%s-%s-%s' "${USER:-user}" "$(basename "$repo_root")" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
		docker_project="gizclaw-e2e-$suffix"
	fi
	validate_docker_project

	local base_image="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-amd64-cn-base}"
	if ! docker image inspect "$base_image" >/dev/null 2>&1; then
		echo "==> build e2e Docker base $base_image"
		docker build -f "$repo_root/build/Dockerfile.cn.base" -t "$base_image" "$repo_root/build"
	fi
	if [[ -z "${GIZCLAW_E2E_DOCKER_SERVER_PORT:-}" ]]; then
		GIZCLAW_E2E_DOCKER_SERVER_PORT="$(pick_free_tcp_port)"
	fi
	export GIZCLAW_E2E_DOCKER_SERVER_PORT

	echo "==> start Docker e2e stack project=$docker_project"
	docker_started=1
	docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" up -d --build

	local server_tcp_port server_udp_port desktop_port server_endpoint desktop_url
	server_tcp_port="$(docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" port --protocol tcp server 9820 | awk -F: '{print $NF}')"
	server_udp_port="$(docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" port --protocol udp server 9820 | awk -F: '{print $NF}')"
	if [[ "$server_tcp_port" != "$server_udp_port" ]]; then
		echo "docker server TCP/UDP port mismatch: tcp=$server_tcp_port udp=$server_udp_port" >&2
		exit 2
	fi
	desktop_port="$(docker compose -p "$docker_project" -f "$docker_dir/docker-compose.yaml" port desktop 4191 | awk -F: '{print $NF}')"
	server_endpoint="127.0.0.1:${server_tcp_port}"
	desktop_url="http://127.0.0.1:${desktop_port}"

	wait_http_ready "http://$server_endpoint/server-info" "docker server" "server"
	wait_docker_ready_file "server" "/tmp/gizclaw-e2e-server-ready" "docker server"
	wait_http_ready "$desktop_url" "docker desktop" "desktop"
	materialize_docker_test_config "$server_endpoint" "$desktop_url"
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

	echo "==> node tests/gizclaw-e2e/js/rpc"
	(cd "$repo_root/tests/gizclaw-e2e/js" && npm run test:rpc)
}

run_desktop_tests() {
	echo "==> go test tests/gizclaw-e2e/desktop"
	(cd "$repo_root" && go test -v -tags gizclaw_e2e -count=1 -timeout "$go_test_timeout" ./tests/gizclaw-e2e/desktop/...)
}

echo "==> build e2e CLI"
"$setup_dir/build.sh" >/dev/null

if [[ "$use_docker" == "1" ]]; then
	start_docker_stack
else
	echo "==> reset e2e data"
	"$setup_dir/reset_data.sh" reset
fi

run_js_rpc_tests
run_desktop_tests
run_pkg "./tests/gizclaw-e2e/cgo/rpc"
run_pkg "./tests/gizclaw-e2e/cgo/chat"
run_pkg "./tests/gizclaw-e2e/cgo/social"
run_pkg "./tests/gizclaw-e2e/go/admin"
run_chat_pkg
run_pkg "./tests/gizclaw-e2e/go/rpc"
run_pkg "./tests/gizclaw-e2e/go/social"
run_pkg "./tests/gizclaw-e2e/cmd/connect"

echo "==> e2e run completed"
