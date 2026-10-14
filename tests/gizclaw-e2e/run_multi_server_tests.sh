#!/usr/bin/env bash
# Multi-server E2E: two GizClaw Servers sharing TiKV with a regional Learner,
# two Edges, one single-node LiveKit and a giztest runner. Runs the Go suite,
# then the SFU giztest scenarios. Provider-backed (TTS/ASR) scenarios only run
# when tests/gizclaw-e2e/.env carries the Volc/Doubao credentials; the
# provider-free audio scenarios always run.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
compose_file="$script_dir/docker/docker-compose.multi-server.yaml"
project="gizclaw-multi-server-${RANDOM}-$$"
bin_dir="$script_dir/testdata/bin"
server_bin="$bin_dir/gizclaw-linux"
test_bin="$bin_dir/multiserver-e2e-linux"
seed_bin="$bin_dir/multiserver-seed-linux"
report_dir="$script_dir/testdata/multi-server/$project"
env_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
giztest_dir="tests/gizclaw-e2e/giztest"
sfu_case="${GIZCLAW_E2E_SFU_CASE:-}"
sfu_repeat="${GIZCLAW_E2E_SFU_REPEAT:-1}"
sfu_delay="${GIZCLAW_E2E_SFU_TERMINAL_DELAY_MS:-0}"
sfu_udp_mode="${GIZCLAW_E2E_LIVEKIT_UDP_MODE:-mux}"
sfu_udp_end="${GIZCLAW_E2E_LIVEKIT_UDP_END:-60000}"
probe_dir=""
compose_files=(-f "$compose_file")
if [[ ! "$sfu_delay" =~ ^(0|[1-9][0-9]{0,4})$ ]] || ((sfu_delay > 10000)); then
  echo "SFU terminal delay must be 0..10000 milliseconds" >&2; exit 2
fi
if [[ ! "$sfu_udp_end" =~ ^[0-9]{5}$ ]] || ((sfu_udp_end <= 50000 || sfu_udp_end > 65535)); then
  echo "SFU UDP range end must be 50001..65535" >&2; exit 2
fi
case "$sfu_udp_mode" in
  mux)
    if [[ -n "${GIZCLAW_E2E_LIVEKIT_UDP_END:-}" ]]; then
      echo "SFU UDP range end requires GIZCLAW_E2E_LIVEKIT_UDP_MODE=range" >&2; exit 2
    fi
    export GIZCLAW_E2E_LIVEKIT_UDP_PORT=7882-7889
    export GIZCLAW_E2E_LIVEKIT_UDP_START=0 GIZCLAW_E2E_LIVEKIT_UDP_END=0
    ;;
  range)
    export GIZCLAW_E2E_LIVEKIT_UDP_PORT=0 GIZCLAW_E2E_LIVEKIT_UDP_START=50000
    export GIZCLAW_E2E_LIVEKIT_UDP_END="$sfu_udp_end"
    ;;
  *) echo "SFU UDP mode must be mux or range" >&2; exit 2 ;;
esac
if ((sfu_delay > 0)) && [[ "$sfu_case" != removal ]]; then
  echo "SFU terminal delay requires the removal case" >&2; exit 2
fi
if [[ -n "$sfu_case" ]]; then
  case "$sfu_case" in startup | removal | logging | logging-join | logging-media | logging-stop) ;; *) echo "unsupported SFU case" >&2; exit 2 ;; esac
  if [[ "${GIZCLAW_E2E_SOCIAL_PING_ONLY:-0}" == 1 ]]; then
    echo "SFU and social notification cases are separate runs" >&2; exit 2
  fi
  if [[ ! "$sfu_repeat" =~ ^[1-9][0-9]?$ ]] || ((sfu_repeat > 64)); then
    echo "SFU repeat must be 1..64" >&2; exit 2
  fi
  compose_files+=(-f "$script_dir/docker/docker-compose.sfu-regression.yaml")
  if [[ "$sfu_case" == logging* ]]; then
    if ((sfu_repeat != 1)); then
      echo "logging pressure uses one deterministic three-Peer task" >&2; exit 2
    fi
    compose_files+=(-f "$script_dir/docker/docker-compose.logging-pressure.yaml")
  fi
  export GIZCLAW_E2E_GIZTEST_PARALLEL="${GIZCLAW_E2E_GIZTEST_PARALLEL:-$sfu_repeat}"
  if [[ ! "$GIZCLAW_E2E_GIZTEST_PARALLEL" =~ ^[1-9][0-9]?$ ]] || ((GIZCLAW_E2E_GIZTEST_PARALLEL > 64)); then
    echo "SFU parallel must be 1..64" >&2; exit 2
  fi
  if [[ "$sfu_case" == startup ]] && ((GIZCLAW_E2E_GIZTEST_PARALLEL < sfu_repeat)); then
    echo "SFU startup barrier requires parallel >= repeat" >&2; exit 2
  fi
  case "${GIZCLAW_E2E_SFU_ACCEPT_REVOCATION:-1}" in 0 | 1) ;; *) echo "SFU accept revocation must be 0 or 1" >&2; exit 2 ;; esac
  export GIZCLAW_E2E_MULTI_REPORT_DIR="$report_dir"
  export GIZCLAW_E2E_GIZTEST_EVIDENCE=full
fi

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
if [[ -n "$sfu_case" ]]; then
  echo "==> provider-free SFU $sfu_case: $sfu_repeat tasks, parallel=$GIZCLAW_E2E_GIZTEST_PARALLEL, terminal delay=${sfu_delay}ms"
elif [[ "${GIZCLAW_E2E_SOCIAL_PING_ONLY:-0}" == 1 ]]; then
  echo "==> provider-free social notification acceptance"
elif [[ -f "$env_file" ]]; then
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
GIZCLAW_E2E_SFU_PG_PASSWORD="$(random_hex 24)"
export GIZCLAW_E2E_SFU_PG_PASSWORD
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
  base_dockerfile="${GIZCLAW_E2E_DOCKER_BASE_DOCKERFILE:-$repo_root/build/gizclaw/Dockerfile.cn.base}"
  echo "==> build missing e2e base image $GIZCLAW_E2E_DOCKER_BASE_IMAGE from $base_dockerfile"
  docker build --platform="$docker_platform" -f "$base_dockerfile" -t "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" "$repo_root/build"
fi

compose() {
  docker compose -p "$project" "${compose_files[@]}" "$@"
}

cleanup() {
  status=$?
  if [[ -f "$report_dir/e2e-run.json" ]]; then
    python3 -B - "$report_dir/e2e-run.json" "$status" <<'PY'
import json, pathlib, sys
path = pathlib.Path(sys.argv[1])
receipt = json.loads(path.read_text())
receipt["exit_status"] = int(sys.argv[2])
path.write_text(json.dumps(receipt, indent=2) + "\n")
PY
  fi
  if [[ -n "${GIZCLAW_E2E_SFU_EVIDENCE_DIR:-}" ]]; then
    mkdir -p "$GIZCLAW_E2E_SFU_EVIDENCE_DIR"
    compose logs --no-color 2>&1 | python3 -B "$script_dir/setup/redact_diagnostics.py" \
      > "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/services.log" || true
    for artifact in go-tests.log e2e-run.json images.txt topology.json sfu-report.json sfu-case.giztest.yaml sfu-run.json; do
      if [[ -f "$report_dir/$artifact" ]]; then
        cp "$report_dir/$artifact" "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/$artifact"
      fi
    done
    if [[ "$sfu_case" == logging* ]]; then
      compose exec -T server-b curl -fsS http://127.0.0.1:9789/profile > "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/blocked-goroutines.txt" || true
      compose exec -T server-b curl -fsS http://127.0.0.1:9789/status > "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/log-store-status.json" || true
      for artifact in "$report_dir"/logging-*; do
        [[ ! -f "$artifact" ]] || cp "$artifact" "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/"
      done
    fi
    if [[ -n "$probe_dir" && -f "$probe_dir/injection.patch" ]]; then
      cp "$probe_dir/injection.patch" "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/injection.patch"
      if [[ -f "$probe_dir/logging-source.json" ]]; then
        cp "$probe_dir/logging-source.json" "$GIZCLAW_E2E_SFU_EVIDENCE_DIR/logging-source.json"
      fi
    fi
  fi
  if (( status != 0 )); then
    compose ps --all >&2 || true
    compose logs --no-color --tail 800 2>&1 \
      | python3 -B "$script_dir/setup/redact_diagnostics.py" >&2 || true
  fi
  compose down --volumes --remove-orphans >/dev/null 2>&1 || true
  if [[ -n "$probe_dir" ]]; then
    rm -rf "$probe_dir"
  fi
  if [[ "${GIZCLAW_E2E_KEEP_BINARIES:-}" != 1 ]]; then
    rm -f "$server_bin" "$test_bin" "$seed_bin"
  fi
}
trap cleanup EXIT

mkdir -p "$bin_dir" "$report_dir"
bash "$script_dir/setup/prepare-tikv.sh"
report_dir="$(cd "$report_dir" && pwd -P)"
export GIZCLAW_E2E_MULTI_REPORT_DIR="$report_dir"
probe_mount=(-e GIZCLAW_E2E_SFU_PROBE_OVERLAY=)
if ((sfu_delay > 0)); then
  probe_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-sfu-probe.XXXXXX")"
  python3 -B "$script_dir/setup/prepare-sfu-probe.py" "$repo_root/pkgs/gizclaw/peer_conn.go" "$probe_dir" "$sfu_delay"
  probe_mount=(-v "$probe_dir:/probe:ro" -e GIZCLAW_E2E_SFU_PROBE_OVERLAY=/probe/overlay.json)
elif [[ "$sfu_case" == logging* ]]; then
  probe_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-logging-probe.XXXXXX")"
  python3 -B "$script_dir/setup/prepare-logging-probe.py" "$repo_root/pkgs/gizlog/logger.go" "$probe_dir" "${GIZCLAW_E2E_LOG_PRESSURE_BASE_REF:-}"
  probe_mount=(-v "$probe_dir:/probe:ro" -e GIZCLAW_E2E_SFU_PROBE_OVERLAY=/probe/overlay.json)
fi
module_cache="$(go env GOMODCACHE)"
build_cache="$(go env GOCACHE)"
if [[ "${GIZCLAW_E2E_REUSE_BINARIES:-}" != 1 || ! -x "$server_bin" || ! -x "$test_bin" || ! -x "$seed_bin" ]] || [[ -n "$sfu_case" && -z "${GIZCLAW_E2E_SFU_BINARY:-}" ]]; then
  docker run --rm --entrypoint /bin/bash \
    -v "$repo_root:/src" \
    -v "$module_cache:/root/go/pkg/mod" \
    -v "$build_cache:/root/.cache/go-build" \
    "${probe_mount[@]}" \
    "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" \
    -lc 'cd /src \
      && npm ci && npm run build:console \
      && probe_args=() \
      && if [[ -n "${GIZCLAW_E2E_SFU_PROBE_OVERLAY:-}" ]]; then probe_args=(-overlay "$GIZCLAW_E2E_SFU_PROBE_OVERLAY"); fi \
      && go build "${probe_args[@]}" -o tests/gizclaw-e2e/testdata/bin/gizclaw-linux ./cmd/gizclaw \
      && go build -o tests/gizclaw-e2e/testdata/bin/multiserver-seed-linux ./tests/gizclaw-e2e/cmd/multiserver-seed \
      && go test -c -tags gizclaw_e2e -o tests/gizclaw-e2e/testdata/bin/multiserver-e2e-linux ./tests/gizclaw-e2e/go/multiserver'
fi
if [[ -n "${GIZCLAW_E2E_SFU_BINARY:-}" ]]; then
  : "${GIZCLAW_E2E_SFU_BINARY_REVISION:?identify the supplied baseline binary revision}"
  cp "$GIZCLAW_E2E_SFU_BINARY" "$server_bin"
fi
if [[ -n "$sfu_case" ]]; then
  prepare_case="$sfu_case"
  [[ "$prepare_case" != logging ]] || prepare_case=logging-join
  docker run --rm --entrypoint node -v "$repo_root:/src" -v "$report_dir:/reports" \
    "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" /src/tests/gizclaw-e2e/setup/prepare-sfu-case.mjs \
    "/src/$giztest_dir/sfu.friend-group.remove-readd.audio-bytes.giztest.yaml" \
    /reports/sfu-case.giztest.yaml "$prepare_case" "$sfu_repeat" "${GIZCLAW_E2E_SFU_ACCEPT_REVOCATION:-1}" "$sfu_delay"
  python3 -B - "$server_bin" "$report_dir/sfu-run.json" "${GIZCLAW_E2E_SFU_BINARY_REVISION:-$(git -C "$repo_root" rev-parse HEAD)}" \
    "$sfu_case" "$sfu_repeat" "$GIZCLAW_E2E_GIZTEST_PARALLEL" "$sfu_delay" "$sfu_udp_mode" "$GIZCLAW_E2E_LIVEKIT_UDP_PORT" "$GIZCLAW_E2E_LIVEKIT_UDP_START" "$GIZCLAW_E2E_LIVEKIT_UDP_END" "$repo_root" <<'PY'
import hashlib, json, pathlib, subprocess, sys
binary, report, revision, case, repeat, parallel, delay, udp_mode, udp_port, udp_start, udp_end, repository = sys.argv[1:]
dirty = subprocess.check_output(["git", "-C", repository, "status", "--porcelain"])
diff = subprocess.check_output(["git", "-C", repository, "diff", "HEAD"])
pathlib.Path(report).write_text(json.dumps({
    "revision": revision, "binary_sha256": hashlib.sha256(pathlib.Path(binary).read_bytes()).hexdigest(),
    "fixture_source_dirty": bool(dirty), "fixture_diff_sha256": hashlib.sha256(diff).hexdigest(),
    "case": case, "tasks": int(repeat), "parallel": int(parallel), "sfu_peers_per_task": 3,
    "terminal_delay_ms": int(delay), "udp_mode": udp_mode, "udp_port": udp_port, "udp_start": int(udp_start), "udp_end": int(udp_end),
    "topology": "2 Servers, 2 Edges, LiveKit, shared TiKV, separate PostgreSQL databases",
}, indent=2) + "\n")
PY
fi

python3 -B - "$repo_root" "$report_dir/e2e-run.json" "$project" "$server_bin" "$test_bin" "$seed_bin" "$GIZCLAW_E2E_SFU_PROVIDER" <<'PY'
import hashlib, json, pathlib, subprocess, sys
repository, report, project, server, tests, seed, provider = sys.argv[1:]
diff = subprocess.check_output(["git", "-C", repository, "diff", "HEAD"])
pathlib.Path(report).write_text(json.dumps({
    "project": project,
    "revision": subprocess.check_output(["git", "-C", repository, "rev-parse", "HEAD"], text=True).strip(),
    "source_diff_sha256": hashlib.sha256(diff).hexdigest(),
    "binary_sha256": {pathlib.Path(p).name: hashlib.sha256(pathlib.Path(p).read_bytes()).hexdigest() for p in [server, tests, seed]},
    "topology": "2 Servers, 2 Edges, LiveKit, PD, center voter, regional Learner with Raft gate; separate local SQL databases",
    "provider_scenarios_enabled": bool(int(provider)),
}, indent=2) + "\n")
PY
compose config --images > "$report_dir/images.txt"
# All application roles share one Dockerfile and the same compiled binaries.
# Build that source snapshot once and tag it for each role.
compose build server-a tikv-init
for role in server-b edge-a edge-b seed giztest tests; do
  docker tag "$project-server-a" "$project-$role"
done
docker tag "$project-tikv-init" "$project-tikv-replication-proxy"
compose up --no-build --wait --wait-timeout 180 tikv-init livekit server-a server-b edge-a edge-b
compose logs --no-color --no-log-prefix tikv-init > "$report_dir/topology.json"

seed_server() {
  local server="$1" token_id="$2" admin_env="$3"
  local build_flags=()
  [[ "$sfu_case" != logging* ]] || build_flags=(--no-deps --pull never)
  # RuntimeProfile, Workflow and RegistrationToken are Server-local catalog:
  # each Server is seeded independently and Peers register with the token of
  # the Server they are homed on. SFU activation never reads another Peer's
  # profile, so nothing catalog-related is shared between the Servers.
  compose run "${build_flags[@]}" --rm -T seed -server "$server" -profile-id multi-server-sfu \
    -token-id "$token_id" -admin-key-env "$admin_env" \
    -monitor-workflow-id workspace-reload-echo | tail -n 1
}
echo "==> seed server-a and server-b catalogs"
GIZCLAW_TEST_REGISTRATION_TOKEN_A="$(seed_server server-a:9820 multi-server-sfu-token-a GIZCLAW_E2E_ADMIN_PRIVATE_KEY_A)"
GIZCLAW_TEST_REGISTRATION_TOKEN_B="$(seed_server server-b:9820 multi-server-sfu-token-b GIZCLAW_E2E_ADMIN_PRIVATE_KEY_B)"
if [[ -z "$GIZCLAW_TEST_REGISTRATION_TOKEN_A" || -z "$GIZCLAW_TEST_REGISTRATION_TOKEN_B" ]]; then
  echo "seeding did not return registration tokens" >&2
  exit 1
fi
export GIZCLAW_TEST_REGISTRATION_TOKEN_A GIZCLAW_TEST_REGISTRATION_TOKEN_B

if [[ -z "$sfu_case" ]]; then
  echo "==> go multi-server tests"
  compose run --rm -T tests \
    -test.run "${GIZCLAW_E2E_TEST_RUN:-.}" -test.parallel 1 | tee "$report_dir/go-tests.log"
fi

if [[ "${GIZCLAW_E2E_SOCIAL_PING_ONLY:-0}" == 1 ]]; then
  compose logs --no-color server-a server-b edge-a edge-b 2>&1 \
    | python3 -B "$script_dir/setup/redact_diagnostics.py" > "$report_dir/social-ping-services.log"
  python3 -B "$script_dir/setup/verify_social_ping_evidence.py" \
    "$report_dir/go-tests.log" "$report_dir/social-ping-services.log" "$report_dir/social-ping-evidence.json"
  echo "==> stop both Edge processes and verify bounded relay failure and local delivery"
  compose stop edge-a edge-b
  compose run --rm --no-deps -T -e GIZCLAW_E2E_EXPECT_NO_RELAY=1 tests \
    -test.run '^TestSocialPingWithoutRelay$' -test.parallel 1 | tee "$report_dir/social-ping-no-relay.log"
  echo "==> social notification Docker e2e completed; service evidence: $report_dir/social-ping-services.log"
  exit 0
fi

for edge in edge-a edge-b; do
  if compose exec -T "$edge" env | grep -Eq '^GIZCLAW_E2E_TIKV_'; then
    echo "$edge unexpectedly received TiKV connector configuration" >&2
    exit 1
  fi
done
if compose logs --no-color edge-a edge-b | grep -Fq 'tikv-pd:2379'; then
  echo "Edge diagnostics exposed the TiKV connector address" >&2
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

if [[ "$sfu_case" == logging ]]; then
  failed=0
  for phase in logging-join logging-media logging-stop logging-recovery; do
    compose exec -T server-b curl -fsS http://127.0.0.1:9789/status > "$report_dir/$phase-before-store-status.json"
    if [[ "$phase" == logging-recovery ]]; then
      cp "$repo_root/$giztest_dir/sfu.friend-group.remove-readd.audio-bytes.giztest.yaml" "$report_dir/$phase.giztest.yaml"
    else
      docker run --rm --entrypoint node -v "$repo_root:/src" -v "$report_dir:/reports" \
        "$GIZCLAW_E2E_DOCKER_BASE_IMAGE" /src/tests/gizclaw-e2e/setup/prepare-sfu-case.mjs \
        "/src/$giztest_dir/sfu.friend-group.remove-readd.audio-bytes.giztest.yaml" \
        "/reports/$phase.giztest.yaml" "$phase" 1 1 0
    fi
    if ! GIZCLAW_E2E_GIZTEST_REPORT_NAME="$phase-report.json" compose run --no-deps --pull never --rm -T giztest "/reports/$phase.giztest.yaml"; then
      failed=1
      print_giztest_failures "$report_dir/$phase-report.json"
    fi
    if [[ "$phase" != logging-recovery ]]; then
      compose exec -T server-b curl -fsS http://127.0.0.1:9789/profile > "$report_dir/$phase-goroutines.txt"
    fi
    compose exec -T server-b curl -fsS http://127.0.0.1:9789/status > "$report_dir/$phase-store-status.json"
  done
  exit "$failed"
fi

scenarios=(
  "$giztest_dir/sfu.friend.cross-server.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.remove-readd.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.reconnect-readd.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.rapid-readd.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.mixed-server-members.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend.delete-readd.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.delete-recreate.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.workspace.isolation-switch.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.workspace.stop-reconnect.audio-bytes.giztest.yaml"
  "$giztest_dir/sfu.friend-group.members-in-room.giztest.yaml"
)
if [[ "$GIZCLAW_E2E_SFU_PROVIDER" == 1 ]]; then
  scenarios+=(
    "$giztest_dir/sfu.friend.cross-server.giztest.yaml"
    "$giztest_dir/sfu.friend-group.cross-server.giztest.yaml"
    "$giztest_dir/sfu.workspace.switch.giztest.yaml"
  )
else
  echo "==> provider-gated SFU scenarios skipped (GIZCLAW_E2E_SFU_PROVIDER=0)"
fi
if [[ -n "$sfu_case" ]]; then
  scenarios=(/reports/sfu-case.giztest.yaml)
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
