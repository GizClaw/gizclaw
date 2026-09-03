#!/usr/bin/env bash
# Multi-server E2E: two GizClaw Servers sharing one Social KV, two Edges, one
# single-node LiveKit and a giztest runner. Runs the Go multi-server suite,
# then the SFU giztest scenarios. Provider-backed (TTS/ASR) scenarios only run
# when tests/gizclaw-e2e/.env carries the Volc/Doubao credentials; the
# provider-free audio scenario always runs.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker/docker-compose.multi-server.yaml"
project="gizclaw-multi-server-${RANDOM}-$$"
bin_dir="$script_dir/testdata/bin"
server_bin="$bin_dir/gizclaw-linux"
test_bin="$bin_dir/multiserver-e2e-linux"
seed_bin="$bin_dir/multiserver-seed-linux"
report_dir="$script_dir/testdata/multi-server"
env_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
giztest_dir="tests/gizclaw-e2e/giztest"

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$script_dir/setup/credentials.sh"

provider_credentials=(
  GIZCLAW_E2E_DOUBAO_APP_ID
  GIZCLAW_E2E_DOUBAO_API_KEY
  GIZCLAW_E2E_DOUBAO_SEARCH_API_KEY
  GIZCLAW_E2E_VOLC_ARK_API_KEY
  GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY_ID
  GIZCLAW_E2E_VOLC_OPENAPI_ACCESS_KEY
)
GIZCLAW_E2E_SFU_PROVIDER=0
if [[ -f "$env_file" ]]; then
  if require_gizclaw_e2e_credentials "$env_file" "${provider_credentials[@]}"; then
    GIZCLAW_E2E_SFU_PROVIDER=1
  else
    echo "==> $env_file lacks complete Volc/Doubao credentials; TTS/ASR scenarios are skipped" >&2
  fi
else
  echo "==> no $env_file; TTS/ASR scenarios are skipped" >&2
fi
export GIZCLAW_E2E_SFU_PROVIDER
for name in "${provider_credentials[@]}"; do
  export "$name=${!name:-}"
done

random_hex() {
  head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'
}
# LiveKit requires secrets of at least 32 characters; both values live only
# for this Compose project.
GIZCLAW_E2E_LIVEKIT_API_KEY="e2e$(random_hex 8)"
GIZCLAW_E2E_LIVEKIT_API_SECRET="$(random_hex 32)"
export GIZCLAW_E2E_LIVEKIT_API_KEY GIZCLAW_E2E_LIVEKIT_API_SECRET
export GIZCLAW_TEST_REGISTRATION_TOKEN_A="" GIZCLAW_TEST_REGISTRATION_TOKEN_B=""

docker_platform="linux/amd64"
case "$(docker info --format '{{.Architecture}}')" in
  aarch64 | arm64) docker_platform="linux/arm64" ;;
esac
platform_slug="${docker_platform//\//-}"
if [[ -z "${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-}" ]]; then
  GIZCLAW_E2E_DOCKER_BASE_IMAGE="gizclaw-go:${platform_slug}-cn-base"
  if [[ "$docker_platform" == "linux/arm64" ]] && docker image inspect gizclaw-go:linux-arm64-cn-base-go1.26.4 >/dev/null 2>&1; then
    GIZCLAW_E2E_DOCKER_BASE_IMAGE="gizclaw-go:linux-arm64-cn-base-go1.26.4"
  fi
fi
export GIZCLAW_E2E_DOCKER_BASE_IMAGE
if ! docker image inspect "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" >/dev/null 2>&1; then
  base_dockerfile="${GIZCLAW_E2E_DOCKER_BASE_DOCKERFILE:-$repo_root/build/Dockerfile.cn.base}"
  echo "==> build missing e2e base image $GIZCLAW_E2E_DOCKER_BASE_IMAGE from $base_dockerfile"
  docker build --platform="$docker_platform" -f "$base_dockerfile" -t "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" "$repo_root/build"
fi

compose() {
  docker compose -p "$project" -f "$compose_file" "$@"
}

cleanup() {
  status=$?
  if (( status != 0 )); then
    compose ps >&2 || true
    compose logs --no-color --tail 800 \
      | grep -E 'peer stream lifecycle|gateway logical session|sfu|livekit|level=(WARN|ERROR)' >&2 || true
    compose logs --no-color --tail 200 livekit >&2 || true
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ "${GIZCLAW_E2E_KEEP_BINARIES:-}" != 1 ]]; then
    rm -f "$server_bin" "$test_bin" "$seed_bin"
  fi
}
trap cleanup EXIT

mkdir -p "$bin_dir" "$report_dir"
module_cache="$(go env GOMODCACHE)"
build_cache="$(go env GOCACHE)"
if [[ "${GIZCLAW_E2E_REUSE_BINARIES:-}" != 1 || ! -x "$server_bin" || ! -x "$test_bin" || ! -x "$seed_bin" ]]; then
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_root:/src" \
    -v "$module_cache:/root/go/pkg/mod" \
    -v "$build_cache:/root/.cache/go-build" \
    "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" \
    -lc 'cd /src \
      && go build -o tests/gizclaw-e2e/testdata/bin/gizclaw-linux ./cmd/gizclaw \
      && go build -o tests/gizclaw-e2e/testdata/bin/multiserver-seed-linux ./tests/gizclaw-e2e/cmd/multiserver-seed \
      && go test -c -tags gizclaw_e2e -o tests/gizclaw-e2e/testdata/bin/multiserver-e2e-linux ./tests/gizclaw-e2e/go/multiserver'
fi

compose up --build --wait redis livekit server-a server-b edge-a edge-b

seed_server() {
  local server="$1" token_id="$2" admin_env="$3"
  # RuntimeProfile, Workflow and RegistrationToken are Server-local catalog:
  # each Server is seeded independently and Peers register with the token of
  # the Server they are homed on. SFU activation never reads another Peer's
  # profile, so nothing catalog-related is shared between the Servers.
  compose run --rm -T seed -server "$server" -profile-id multi-server-sfu \
    -token-id "$token_id" -admin-key-env "$admin_env" | tail -n 1
}
echo "==> seed server-a and server-b catalogs"
GIZCLAW_TEST_REGISTRATION_TOKEN_A="$(seed_server server-a:9820 multi-server-sfu-token-a GIZCLAW_E2E_ADMIN_PRIVATE_KEY_A)"
GIZCLAW_TEST_REGISTRATION_TOKEN_B="$(seed_server server-b:9820 multi-server-sfu-token-b GIZCLAW_E2E_ADMIN_PRIVATE_KEY_B)"
if [[ -z "$GIZCLAW_TEST_REGISTRATION_TOKEN_A" || -z "$GIZCLAW_TEST_REGISTRATION_TOKEN_B" ]]; then
  echo "seeding did not return registration tokens" >&2
  exit 1
fi
export GIZCLAW_TEST_REGISTRATION_TOKEN_A GIZCLAW_TEST_REGISTRATION_TOKEN_B

echo "==> go multi-server tests"
compose run --rm -T tests \
  -test.run "${GIZCLAW_E2E_TEST_RUN:-.}" -test.parallel 1

if compose exec -T redis redis-cli --scan --pattern '*runs*' | grep -q .; then
  echo "shared Redis unexpectedly contains PeerRun keys" >&2
  exit 1
fi

for edge in edge-a edge-b; do
  if compose exec -T "$edge" env \
    | grep -Eq '^(REDIS_URL|REDIS_DSN|GIZCLAW_E2E_REDIS_DSN)='; then
    echo "$edge unexpectedly received Redis credentials" >&2
    exit 1
  fi
done

if compose logs --no-color edge-a edge-b | grep -Fq 'redis://redis:6379/0'; then
  echo "Edge diagnostics exposed the Redis DSN" >&2
  exit 1
fi

print_giztest_failures() {
  local report="$1"
  if [[ ! -f "$report" ]]; then
    echo "giztest report $report was not written" >&2
    return
  fi
  python3 - "$report" <<'PY' >&2
import json, sys
report = json.load(open(sys.argv[1], encoding="utf-8"))
print(f"giztest report status={report.get('status')} duration_ms={report.get('duration_ms')}")
for task in report.get("tasks", []):
    if task.get("status") == "passed":
        continue
    print(f"task {task.get('name')} [{task.get('task_id')}] status={task.get('status')} error={task.get('error', '')}")
    for step in task.get("steps", []) + task.get("cleanup", []):
        if step.get("status") == "passed":
            continue
        print(f"  step {step.get('id')} {step.get('operation')} client={step.get('client', '')} status={step.get('status')} error={step.get('error', '')}")
        for child in step.get("children", []):
            if child.get("status") == "passed":
                continue
            print(f"    child {child.get('id')} {child.get('operation')} client={child.get('client', '')} status={child.get('status')} error={child.get('error', '')}")
PY
}

scenarios=("$giztest_dir/sfu.friend.cross-server.audio-bytes.giztest.yaml")
if [[ "$GIZCLAW_E2E_SFU_PROVIDER" == 1 ]]; then
  scenarios+=(
    "$giztest_dir/sfu.friend.cross-server.giztest.yaml"
    "$giztest_dir/sfu.friend-group.cross-server.giztest.yaml"
    "$giztest_dir/sfu.workspace.switch.giztest.yaml"
  )
else
  echo "==> provider-gated SFU scenarios skipped (GIZCLAW_E2E_SFU_PROVIDER=0)"
fi
report_name="sfu-report.json"
rm -f "$report_dir/$report_name"
echo "==> giztest SFU scenarios: ${scenarios[*]}"
if ! GIZCLAW_E2E_GIZTEST_REPORT_NAME="$report_name" compose run --rm -T giztest "${scenarios[@]}"; then
  echo "SFU giztest scenarios failed; report: $report_dir/$report_name" >&2
  print_giztest_failures "$report_dir/$report_name"
  exit 1
fi
echo "==> multi-server e2e completed (provider scenarios: $GIZCLAW_E2E_SFU_PROVIDER)"
