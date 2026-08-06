#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact_root="$script_dir/testdata/gateway-capacity-extended"
artifact_base="${GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR:-$artifact_root}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
capacity_image_tag="$(printf '%s' "$run_id" | tr '[:upper:]' '[:lower:]')"
capacity_image="${GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE:-gizclaw-capacity:$capacity_image_tag}"
export GIZCLAW_E2E_GATEWAY_CAPACITY_IMAGE="$capacity_image"
export GIZCLAW_E2E_DOCKER_RETAIN_LOCAL_IMAGES=1
current_env=""
runtime_state=""
coturn_monitor_pid=""
coturn_monitor_stop=""
coturn_a_container_id=""
coturn_b_container_id=""
gateway_workload_pid=""
capacity_stack_start_pid=""
gateway_gomaxprocs="${GIZCLAW_E2E_GATEWAY_GOMAXPROCS:-8}"
gateway_gogc="${GIZCLAW_E2E_GATEWAY_GOGC:-100}"
gateway_dial_timeout="${GIZCLAW_E2E_GATEWAY_DIAL_TIMEOUT:-20s}"
gateway_ping_timeout="${GIZCLAW_E2E_GATEWAY_PING_TIMEOUT:-28s}"
gateway_speed_bytes="${GIZCLAW_E2E_GATEWAY_SPEED_BYTES:-0}"
gateway_speed_baseline_bytes="${GIZCLAW_E2E_GATEWAY_SPEED_BASELINE_BYTES:-0}"
gateway_speed_timeout="${GIZCLAW_E2E_GATEWAY_SPEED_TIMEOUT:-2m}"
gateway_min_speed_aggregate_ratio="${GIZCLAW_E2E_GATEWAY_MIN_SPEED_AGGREGATE_RATIO:-0}"
gateway_min_upload_aggregate_mbps="${GIZCLAW_E2E_GATEWAY_MIN_UPLOAD_AGGREGATE_MBPS:-0}"
gateway_min_download_aggregate_mbps="${GIZCLAW_E2E_GATEWAY_MIN_DOWNLOAD_AGGREGATE_MBPS:-0}"
gateway_min_final_speed_retention="${GIZCLAW_E2E_GATEWAY_MIN_FINAL_SPEED_RETENTION:-0}"
gateway_min_establishment_rate="${GIZCLAW_E2E_GATEWAY_MIN_ESTABLISHMENT_RATE:-0}"
gateway_max_dial_p95="${GIZCLAW_E2E_GATEWAY_MAX_DIAL_P95:-0}"
gateway_max_dial_p99="${GIZCLAW_E2E_GATEWAY_MAX_DIAL_P99:-0}"
gateway_concurrency="${GIZCLAW_E2E_GATEWAY_CONCURRENCY:-512}"
gateway_required_upstreams_per_edge="${GIZCLAW_E2E_GATEWAY_REQUIRED_UPSTREAMS_PER_EDGE:-4}"
gateway_upstream_path="${GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH:-relay}"
gateway_prebuilt="${GIZCLAW_E2E_GATEWAY_PREBUILT:-0}"
gateway_cleanup_timeout="${GIZCLAW_E2E_GATEWAY_CLEANUP_TIMEOUT:-30s}"
gateway_post_start_settle_seconds="${GIZCLAW_E2E_GATEWAY_POST_START_SETTLE_SECONDS:-0}"
gateway_retain_image_on_failure="${GIZCLAW_E2E_GATEWAY_RETAIN_IMAGE_ON_FAILURE:-0}"
case "$gateway_upstream_path" in
  direct | relay) ;;
  *)
    echo "GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH must be direct or relay" >&2
    exit 2
    ;;
esac

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"
for required_command in jq python3; do
  if ! command -v "$required_command" >/dev/null 2>&1; then
    echo "extended capacity requires $required_command to write Coturn evidence" >&2
    exit 2
  fi
done

mkdir -p "$artifact_root/direct" "$artifact_root/relay" "$script_dir/testdata/docker" "$script_dir/testdata/bin"
artifact_root="$(cd "$artifact_root" && pwd -P)"
if [[ -e "$artifact_base" ]]; then
  artifact_base="$(cd "$artifact_base" && pwd -P)"
else
  artifact_parent="$(cd "$(dirname "$artifact_base")" && pwd -P)"
  artifact_base="$artifact_parent/$(basename "$artifact_base")"
fi
case "$artifact_base/" in
  "$artifact_root/"*) ;;
  *)
    echo "extended capacity artifacts must stay below $artifact_root: $artifact_base" >&2
    exit 2
    ;;
esac
mkdir -p "$artifact_base"
artifact_base="$(cd "$artifact_base" && pwd -P)"
artifact_set="$artifact_base/$run_id"
runs_dir="$artifact_set/runs"
bin_dir="$artifact_set/bin"
gateway_bin="${GIZCLAW_E2E_GATEWAY_CAPACITY_BIN:-$bin_dir/gateway-capacity}"
runtime_state="$(mktemp -d "$script_dir/testdata/docker/gateway-capacity.XXXXXX")"
mkdir -p "$runs_dir" "$bin_dir"

cleanup_current() {
  local cleanup_pid cleanup_status elapsed_seconds last_heartbeat_seconds started_seconds
  if [[ -z "$current_env" || ! -f "$current_env" ]]; then
    return 0
  fi
  echo "==> capacity stack cleanup heartbeat: status=started env=$current_env"
  started_seconds="$SECONDS"
  last_heartbeat_seconds=0
  GIZCLAW_E2E_DOCKER_ENV="$current_env" bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 &
  cleanup_pid="$!"
  while kill -0 "$cleanup_pid" 2>/dev/null; do
    sleep 1
    elapsed_seconds=$((SECONDS - started_seconds))
    if kill -0 "$cleanup_pid" 2>/dev/null && ((elapsed_seconds >= last_heartbeat_seconds + 15)); then
      echo "==> capacity stack cleanup heartbeat: status=running elapsed_seconds=$elapsed_seconds env=$current_env"
      last_heartbeat_seconds="$elapsed_seconds"
    fi
  done
  cleanup_status=0
  wait "$cleanup_pid" || cleanup_status="$?"
  elapsed_seconds=$((SECONDS - started_seconds))
  if ((cleanup_status != 0)); then
    echo "==> capacity stack cleanup heartbeat: status=failed exit_code=$cleanup_status elapsed_seconds=$elapsed_seconds env=$current_env" >&2
    return "$cleanup_status"
  fi
  echo "==> capacity stack cleanup heartbeat: status=completed elapsed_seconds=$elapsed_seconds env=$current_env"
  rm -f "$current_env"
  current_env=""
  coturn_a_container_id=""
  coturn_b_container_id=""
}

cleanup_on_exit() {
  local status="$?"
  if [[ -n "$capacity_stack_start_pid" ]] && kill -0 "$capacity_stack_start_pid" 2>/dev/null; then
    kill -TERM "$capacity_stack_start_pid" 2>/dev/null || true
    wait "$capacity_stack_start_pid" 2>/dev/null || true
    capacity_stack_start_pid=""
  fi
  if [[ -n "$gateway_workload_pid" ]] && kill -0 "$gateway_workload_pid" 2>/dev/null; then
    kill -TERM "$gateway_workload_pid" 2>/dev/null || true
    wait "$gateway_workload_pid" 2>/dev/null || true
    gateway_workload_pid=""
  fi
  if ! stop_coturn_monitor; then
    echo "failed to stop the active Coturn allocation monitor" >&2
    status=1
  fi
  if ! cleanup_current; then
    echo "failed to clean the active gateway-capacity Docker project; env=$current_env" >&2
    status=1
  fi
  if ((status != 0)) && [[ "$gateway_retain_image_on_failure" == "1" ]]; then
    echo "==> capacity image cleanup heartbeat: status=retained-for-retry image=$capacity_image"
  elif ! cleanup_capacity_image; then
    status=1
  fi
  rmdir "$runtime_state" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup_on_exit EXIT

docker_daemon_responsive() {
  python3 - <<'PY'
import subprocess

try:
    subprocess.run(
        ["docker", "info"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        timeout=3,
        check=True,
    )
except (OSError, subprocess.SubprocessError):
    raise SystemExit(1)
PY
}

cleanup_capacity_image() {
  local cleanup_pid cleanup_status elapsed_seconds last_heartbeat_seconds started_seconds
  echo "==> capacity image cleanup heartbeat: status=started image=$capacity_image"
  (
    if docker image inspect "$capacity_image" >/dev/null 2>&1; then
      docker image rm "$capacity_image" >/dev/null
    fi
  ) &
  cleanup_pid="$!"
  started_seconds="$SECONDS"
  last_heartbeat_seconds=0
  while kill -0 "$cleanup_pid" 2>/dev/null; do
    sleep 1
    elapsed_seconds=$((SECONDS - started_seconds))
    if kill -0 "$cleanup_pid" 2>/dev/null && ((elapsed_seconds >= last_heartbeat_seconds + 15)); then
      echo "==> capacity image cleanup heartbeat: status=running elapsed_seconds=$elapsed_seconds image=$capacity_image"
      last_heartbeat_seconds="$elapsed_seconds"
    fi
  done
  cleanup_status=0
  wait "$cleanup_pid" || cleanup_status="$?"
  elapsed_seconds=$((SECONDS - started_seconds))
  if ((cleanup_status != 0)); then
    echo "==> capacity image cleanup heartbeat: status=failed exit_code=$cleanup_status elapsed_seconds=$elapsed_seconds image=$capacity_image" >&2
    return "$cleanup_status"
  fi
  echo "==> capacity image cleanup heartbeat: status=completed elapsed_seconds=$elapsed_seconds image=$capacity_image"
}

start_capacity_stack() {
  local project="$1"
  local env_path="$2"
  local topology_flag="$3"
  local start_status elapsed_seconds last_heartbeat_seconds started_seconds daemon_status
  env GIZCLAW_E2E_DOCKER_PROJECT="$project" \
    GIZCLAW_E2E_DOCKER_ENV="$env_path" \
    bash "$setup_dir/docker-compose-up.sh" "$topology_flag" &
  capacity_stack_start_pid="$!"
  started_seconds="$SECONDS"
  last_heartbeat_seconds=0
  while kill -0 "$capacity_stack_start_pid" 2>/dev/null; do
    sleep 1
    elapsed_seconds=$((SECONDS - started_seconds))
    if kill -0 "$capacity_stack_start_pid" 2>/dev/null && ((elapsed_seconds >= last_heartbeat_seconds + 15)); then
      daemon_status=unresponsive
      if docker_daemon_responsive; then
        daemon_status=responsive
      fi
      echo "==> capacity stack startup heartbeat: status=running docker_daemon=$daemon_status elapsed_seconds=$elapsed_seconds project=$project"
      last_heartbeat_seconds="$elapsed_seconds"
    fi
  done
  start_status=0
  wait "$capacity_stack_start_pid" || start_status="$?"
  capacity_stack_start_pid=""
  elapsed_seconds=$((SECONDS - started_seconds))
  if ((start_status != 0)); then
    echo "==> capacity stack startup heartbeat: status=failed exit_code=$start_status elapsed_seconds=$elapsed_seconds project=$project" >&2
    return "$start_status"
  fi
  echo "==> capacity stack startup heartbeat: status=completed elapsed_seconds=$elapsed_seconds project=$project"
}

read_gateway_limit() {
  local key="$1"
  awk -v key="$key:" '$1 == key { print $2; found = 1; exit } END { if (!found) exit 1 }' \
    "$script_dir/testdata/edge-workspace/config.yaml.template"
}

verify_capacity_stack_running() {
  local service container_id health pending_health=0
  for service in turn server edge edge2 coturn-a coturn-b; do
    container_id="$(docker ps -q \
      --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" \
      --filter "label=com.docker.compose.service=$service")"
    if [[ -z "$container_id" || "$container_id" == *$'\n'* ]]; then
      echo "capacity stack lost service during post-start settle: service=$service container=$container_id" >&2
      return 1
    fi
    health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container_id")"
    if [[ "$health" == "starting" ]]; then
      pending_health=1
      continue
    fi
    if [[ "$health" != "healthy" ]]; then
      echo "capacity stack service lost health during post-start settle: service=$service health=$health" >&2
      return 1
    fi
  done
  if [[ "$pending_health" == "1" ]]; then
    return 2
  fi
}

verify_capacity_service_image() {
  local expected_image_id service container_id actual_image_id
  expected_image_id="$(docker image inspect --format '{{.Id}}' "$capacity_image")"
  for service in server edge edge2; do
    container_id="$(resolve_compose_container_id "$service")"
    actual_image_id="$(docker inspect --format '{{.Image}}' "$container_id")"
    if [[ "$actual_image_id" != "$expected_image_id" ]]; then
      echo "capacity service image mismatch: service=$service expected=$expected_image_id actual=$actual_image_id" >&2
      return 1
    fi
  done
  echo "==> capacity service image verified: services=server,edge,edge2 image=$capacity_image image_id=$expected_image_id"
}

wait_capacity_post_start_settle() {
  local remaining="$gateway_post_start_settle_seconds" health_status
  if [[ "$remaining" == "0" ]]; then
    return 0
  fi
  while ((remaining > 0)); do
    health_status=0
    verify_capacity_stack_running || health_status="$?"
    case "$health_status" in
      0) echo "==> capacity post-start settle heartbeat: status=healthy services=6 remaining_seconds=$remaining image=$capacity_image" ;;
      2) echo "==> capacity post-start settle heartbeat: status=starting services=6 remaining_seconds=$remaining image=$capacity_image" ;;
      *) return "$health_status" ;;
    esac
    sleep 15
    remaining=$((remaining - 15))
  done
  health_status=0
  verify_capacity_stack_running || health_status="$?"
  if [[ "$health_status" == "2" ]]; then
    echo "capacity stack services did not become healthy during post-start settle" >&2
    return 1
  fi
  if ((health_status != 0)); then
    return "$health_status"
  fi
  echo "==> capacity post-start settle heartbeat: status=ready services=6 remaining_seconds=0 image=$capacity_image"
}

resolve_capacity_edge_endpoint() {
  local service="$1"
  local published_endpoint="$2"
  local container_id container_ip direct_endpoint
  container_id="$(docker ps -q \
    --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" \
    --filter "label=com.docker.compose.service=$service")"
  if [[ -n "$container_id" ]]; then
    container_ip="$(docker inspect --format '{{range .NetworkSettings.Networks}}{{println .IPAddress}}{{end}}' "$container_id" 2>/dev/null | awk 'NF { print; exit }')"
    direct_endpoint="${container_ip:+$container_ip:9821}"
    if [[ -n "$direct_endpoint" ]] && curl -fsS --connect-timeout 1 --max-time 2 "http://$direct_endpoint/server-info" >/dev/null 2>&1; then
      echo "==> capacity signaling endpoint: service=$service direct=$direct_endpoint" >&2
      printf '%s\n' "$direct_endpoint"
      return 0
    fi
  fi
  echo "==> capacity signaling endpoint: service=$service published=$published_endpoint (direct unavailable)" >&2
  printf '%s\n' "$published_endpoint"
}

read_coturn_metrics() {
  local service="$1"
  local container_id output
  case "$service" in
    coturn-a) container_id="$coturn_a_container_id" ;;
    coturn-b) container_id="$coturn_b_container_id" ;;
    *) echo "unknown Coturn service: $service" >&2; return 2 ;;
  esac
  if [[ -z "$container_id" ]]; then
    echo "Coturn container ID is unavailable for service=$service" >&2
    return 1
  fi
  output="$(docker exec "$container_id" bash -lc '
      exec 3<>/dev/tcp/127.0.0.1/9641
      printf "GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n" >&3
      cat <&3
    ')"
  awk '
    $1 == "turn_total_allocations" || index($1, "turn_total_allocations{") == 1 { allocations += $2 }
    $1 == "turn_total_traffic_rcvb" || index($1, "turn_total_traffic_rcvb{") == 1 { received += $2 }
    $1 == "turn_total_traffic_sentb" || index($1, "turn_total_traffic_sentb{") == 1 { sent += $2 }
    END { printf "%.0f %.0f %.0f\n", allocations, received, sent }
  ' <<<"$output"
}

resolve_compose_container_id() {
  local service="$1"
  local container_id
  container_id="$(docker ps -q \
    --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" \
    --filter "label=com.docker.compose.service=$service")"
  if [[ -z "$container_id" || "$container_id" == *$'\n'* ]]; then
    echo "expected one running container for service=$service, got: $container_id" >&2
    return 1
  fi
  printf '%s\n' "$container_id"
}

numeric_sum() {
  awk -v left="$1" -v right="$2" 'BEGIN { printf "%.0f\n", left + right }'
}

numeric_greater() {
  awk -v value="$1" -v baseline="$2" 'BEGIN { exit !(value > baseline) }'
}

stream_coturn_metrics() {
  local container_id="$1"
  local stop_file="$2"
  local initial_delay_milliseconds="$3"
  docker exec -i "$container_id" bash -s -- "$stop_file" "$initial_delay_milliseconds" <<'EOF'
set -euo pipefail
stop_file="$1"
initial_delay_milliseconds="$2"
if ((initial_delay_milliseconds > 0)); then
  initial_delay_seconds="$(awk -v milliseconds="$initial_delay_milliseconds" 'BEGIN { printf "%.3f", milliseconds / 1000 }')"
  sleep "$initial_delay_seconds"
fi
while [[ ! -e "$stop_file" ]]; do
  iteration_started_nanoseconds="${EPOCHREALTIME/./}000"
  output="$({
    exec 3<>/dev/tcp/127.0.0.1/9641
    printf "GET /metrics HTTP/1.0\r\nHost: localhost\r\n\r\n" >&3
    cat <&3
  })"
  read -r allocations received sent < <(awk '
    $1 == "turn_total_allocations" || index($1, "turn_total_allocations{") == 1 { allocations += $2 }
    $1 == "turn_total_traffic_rcvb" || index($1, "turn_total_traffic_rcvb{") == 1 { received += $2 }
    $1 == "turn_total_traffic_sentb" || index($1, "turn_total_traffic_sentb{") == 1 { sent += $2 }
    END { printf "%.0f %.0f %.0f\n", allocations, received, sent }
  ' <<<"$output")
  sampled_at_nanoseconds="${EPOCHREALTIME/./}000"
  printf '%s %s %s %s\n' "$sampled_at_nanoseconds" "$allocations" "$received" "$sent"
  now_nanoseconds="${EPOCHREALTIME/./}000"
  sleep_milliseconds=$(((iteration_started_nanoseconds + 1000000000 - now_nanoseconds) / 1000000))
  if ((sleep_milliseconds > 0)); then
    sleep_seconds="$(awk -v milliseconds="$sleep_milliseconds" 'BEGIN { printf "%.3f", milliseconds / 1000 }')"
    sleep "$sleep_seconds"
  fi
done
EOF
}

signal_coturn_monitor_stop() {
  local stop_file="$1"
  local a_pid b_pid a_status=0 b_status=0
  docker exec "$coturn_a_container_id" touch "$stop_file" &
  a_pid="$!"
  docker exec "$coturn_b_container_id" touch "$stop_file" &
  b_pid="$!"
  wait "$a_pid" || a_status="$?"
  wait "$b_pid" || b_status="$?"
  ((a_status == 0 && b_status == 0))
}

merge_coturn_metric_streams() {
  local expected="$1"
  local a_output="$2"
  local a_redundant_output="$3"
  local b_output="$4"
  local b_redundant_output="$5"
  local output="$6"
  python3 - "$expected" "$a_output" "$a_redundant_output" "$b_output" "$b_redundant_output" "$output" <<'PY'
import datetime
import json
import pathlib
import sys

expected = int(sys.argv[1])
member_paths = [
    [pathlib.Path(sys.argv[2]), pathlib.Path(sys.argv[3])],
    [pathlib.Path(sys.argv[4]), pathlib.Path(sys.argv[5])],
]
output = pathlib.Path(sys.argv[6])

def read_samples(path):
    samples = []
    for line in path.read_text().splitlines():
        timestamp, allocations, received, sent = (int(value) for value in line.split())
        samples.append((timestamp, allocations, received, sent))
    return samples

if expected % 2 != 0:
    raise SystemExit("expected Coturn allocation count must divide evenly between members")

expected_member = expected // 2
members = []
for member_index, paths in enumerate(member_paths):
    member_samples = []
    for path in paths:
        samples = read_samples(path)
        if not samples:
            raise SystemExit(f"Coturn metric stream produced no samples: path={path}")
        previous = None
        for timestamp, allocations, _, _ in samples:
            if previous is not None and timestamp <= previous:
                raise SystemExit(
                    f"Coturn metric stream is not monotonic: path={path} "
                    f"previous={previous} actual={timestamp}"
                )
            previous = timestamp
            if allocations != expected_member:
                raise SystemExit(
                    f"Coturn live allocations changed: member={member_index} "
                    f"expected={expected_member} actual={allocations} timestamp={timestamp}"
                )
        member_samples.extend(samples)
    member_samples.sort()
    previous = None
    for timestamp, allocations, _, _ in member_samples:
        if allocations != expected_member:
            raise SystemExit(
                f"Coturn live allocations changed: member={member_index} "
                f"expected={expected_member} actual={allocations} timestamp={timestamp}"
            )
        if previous is not None and timestamp - previous > 2_100_000_000:
            raise SystemExit(
                f"Coturn member metric stream exceeds the 2.1-second gap: "
                f"member={member_index} previous={previous} actual={timestamp} "
                f"gap_nanoseconds={timestamp - previous}"
            )
        previous = timestamp
    members.append(member_samples)

paired = []
a_index = 0
b_index = 0
while a_index < len(members[0]) and b_index < len(members[1]):
    a_sample = members[0][a_index]
    b_sample = members[1][b_index]
    difference = a_sample[0] - b_sample[0]
    if abs(difference) <= 1_000_000_000:
        paired.append((a_sample, b_sample))
        a_index += 1
        b_index += 1
    elif difference < 0:
        a_index += 1
    else:
        b_index += 1
if not paired:
    raise SystemExit("Coturn member metric streams have no overlapping samples")

with output.open("w") as stream:
    for a_sample, b_sample in paired:
        timestamp = max(a_sample[0], b_sample[0])
        sampled_at = datetime.datetime.fromtimestamp(
            timestamp / 1_000_000_000, datetime.timezone.utc
        ).strftime("%Y-%m-%dT%H:%M:%SZ")
        value = {
            "sampled_at": sampled_at,
            "sampled_at_unix_milliseconds": timestamp // 1_000_000,
            "total_allocations": a_sample[1] + b_sample[1],
            "coturn_a": {
                "allocations": a_sample[1],
                "received_bytes": a_sample[2],
                "sent_bytes": a_sample[3],
            },
            "coturn_b": {
                "allocations": b_sample[1],
                "received_bytes": b_sample[2],
                "sent_bytes": b_sample[3],
            },
        }
        stream.write(json.dumps(value, separators=(",", ":")) + "\n")
PY
}

monitor_coturn_allocations() (
  local expected="$1"
  local output="$2"
  local stop_file="$3"
  local a_output="${output}.coturn-a.tmp" a_redundant_output="${output}.coturn-a-redundant.tmp"
  local b_output="${output}.coturn-b.tmp" b_redundant_output="${output}.coturn-b-redundant.tmp"
  local stream_failed=false index status
  local -a pids=() labels=(coturn-a coturn-a-redundant coturn-b coturn-b-redundant)
  trap 'rm -f "$a_output" "$a_redundant_output" "$b_output" "$b_redundant_output"' EXIT
  : >"$a_output"
  : >"$a_redundant_output"
  : >"$b_output"
  : >"$b_redundant_output"
  stream_coturn_metrics "$coturn_a_container_id" "$stop_file" 0 >"$a_output" &
  pids+=("$!")
  stream_coturn_metrics "$coturn_a_container_id" "$stop_file" 500 >"$a_redundant_output" &
  pids+=("$!")
  stream_coturn_metrics "$coturn_b_container_id" "$stop_file" 0 >"$b_output" &
  pids+=("$!")
  stream_coturn_metrics "$coturn_b_container_id" "$stop_file" 500 >"$b_redundant_output" &
  pids+=("$!")
  while kill -0 "${pids[0]}" 2>/dev/null && kill -0 "${pids[1]}" 2>/dev/null && \
    kill -0 "${pids[2]}" 2>/dev/null && kill -0 "${pids[3]}" 2>/dev/null; do
    sleep 0.1
  done
  signal_coturn_monitor_stop "$stop_file" || true
  for index in "${!pids[@]}"; do
    status=0
    wait "${pids[$index]}" || status="$?"
    if ((status != 0)); then
      echo "Coturn member metric stream failed: ${labels[$index]}=$status" >&2
      stream_failed=true
    fi
  done
  if [[ "$stream_failed" == true ]]; then
    return 1
  fi
  merge_coturn_metric_streams "$expected" "$a_output" "$a_redundant_output" \
    "$b_output" "$b_redundant_output" "$output"
)

stop_coturn_monitor() {
  local status=0
  if [[ -z "$coturn_monitor_pid" ]]; then
    return 0
  fi
  if ! signal_coturn_monitor_stop "$coturn_monitor_stop"; then
    status=1
  fi
  wait "$coturn_monitor_pid" || status=$?
  coturn_monitor_pid=""
  coturn_monitor_stop=""
  return "$status"
}

wait_coturn_monitor_ready() {
  local output="$1"
  local deadline=$((SECONDS + 10))
  local status=0
  while ((SECONDS <= deadline)); do
    if [[ -s "${output}.coturn-a.tmp" && -s "${output}.coturn-a-redundant.tmp" && \
      -s "${output}.coturn-b.tmp" && -s "${output}.coturn-b-redundant.tmp" ]]; then
      return 0
    fi
    if ! kill -0 "$coturn_monitor_pid" 2>/dev/null; then
      wait "$coturn_monitor_pid" || status=$?
      coturn_monitor_pid=""
      echo "Coturn live-allocation monitor exited before its first sample: status=$status" >&2
      return 1
    fi
    sleep 0.1
  done
  echo "Coturn live-allocation monitor did not produce samples for both members within 10 seconds" >&2
  return 1
}

validate_coturn_live_samples() {
  local expected="$1"
  local output="$2"
  if jq -es --argjson expected "$expected" '
    length > 0 and
    all(.[]; .total_allocations == $expected) and
    ([.[].sampled_at_unix_milliseconds] as $timestamps |
      all(range(1; $timestamps | length);
        ($timestamps[.] >= $timestamps[. - 1]) and
        ($timestamps[.] - $timestamps[. - 1] <= 2100)))
  ' "$output" >/dev/null; then
    return 0
  fi
  echo "Coturn live-allocation samples are empty, exceed a 2.1-second gap, or differ from expected=$expected" >&2
  return 1
}

wait_coturn_allocations_zero() {
  local deadline=$((SECONDS + 15))
  local a_alloc b_alloc
  while ((SECONDS <= deadline)); do
    read -r a_alloc _ _ < <(read_coturn_metrics coturn-a)
    read -r b_alloc _ _ < <(read_coturn_metrics coturn-b)
    if [[ "$a_alloc" == "0" && "$b_alloc" == "0" ]]; then
      return 0
    fi
    sleep 0.1
  done
  echo "Coturn allocations did not return to zero within 15 seconds: coturn-a=$a_alloc coturn-b=$b_alloc" >&2
  return 1
}

wait_coturn_allocation_count() {
  local expected="$1"
  local deadline=$((SECONDS + 30))
  local a_alloc=unknown a_recv=unknown a_sent=unknown
  local b_alloc=unknown b_recv=unknown b_sent=unknown
  local total
  while ((SECONDS <= deadline)); do
    if read -r a_alloc a_recv a_sent < <(read_coturn_metrics coturn-a) &&
      read -r b_alloc b_recv b_sent < <(read_coturn_metrics coturn-b); then
      total="$(numeric_sum "$a_alloc" "$b_alloc")"
      if [[ "$total" == "$expected" ]]; then
        printf '%s %s %s %s %s %s\n' \
          "$a_alloc" "$a_recv" "$a_sent" "$b_alloc" "$b_recv" "$b_sent"
        return 0
      fi
    fi
    sleep 0.1
  done
  echo "Coturn allocations did not reach $expected within 30 seconds: coturn-a=$a_alloc coturn-b=$b_alloc" >&2
  return 1
}

max_sessions_per_edge="$(read_gateway_limit max-sessions)"
max_upstreams_per_edge="$(read_gateway_limit max-upstreams)"
max_sessions_per_upstream="$(read_gateway_limit sessions-per-upstream)"
if [[ "$max_sessions_per_edge" != "30000" || "$max_upstreams_per_edge" != "16" || "$max_sessions_per_upstream" != "2048" ]]; then
  echo "extended capacity requires Edge limits 30000/16/2048; configured $max_sessions_per_edge/$max_upstreams_per_edge/$max_sessions_per_upstream" >&2
  exit 2
fi
if [[ "$gateway_prebuilt" == "1" ]]; then
  if [[ ! -x "$script_dir/testdata/bin/gizclaw-linux" || ! -x "$gateway_bin" ]]; then
    echo "prebuilt capacity binaries are missing" >&2
    exit 2
  fi
else
  echo "==> build Linux CGO e2e CLI and host gateway-capacity runner"
  bash "$setup_dir/build-linux-cgo.sh"
  (cd "$repo_root" && go build -o "$gateway_bin" ./tests/gizclaw-e2e/gateway-capacity)
fi
export GIZCLAW_E2E_GATEWAY_LINUX_PREBUILT=1

run_case() {
  local scenario="$1"
  local sessions="$2"
  local ramp="$3"
  local hold="$4"
  local repetition="$5"
  local soak="$6"
  local project_slug artifact coturn_artifact coturn_live_artifact path_artifact capacity_edge_endpoint capacity_edge2_endpoint
  local topology_flag expected_allocations edge_log edge2_log edge_id edge2_id
  local workload_status monitor_status
  local before_a_alloc before_a_recv before_a_sent before_b_alloc before_b_recv before_b_sent
  local after_a_alloc after_a_recv after_a_sent after_b_alloc after_b_recv after_b_sent
  local cleanup_a_alloc cleanup_a_recv cleanup_a_sent cleanup_b_alloc cleanup_b_recv cleanup_b_sent
  project_slug="$(printf '%s-%s-%s' "$run_id" "$scenario" "$repetition" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
  artifact="$runs_dir/${scenario}-run-${repetition}.json"
  current_env="$runtime_state/${scenario}-run-${repetition}.env"
  topology_flag="--gateway-capacity"
  expected_allocations=10
  if [[ "$gateway_upstream_path" == "direct" ]]; then
    topology_flag="--gateway-capacity-direct"
    expected_allocations=0
  fi
  echo "==> start fresh capacity stack: path=$gateway_upstream_path scenario=$scenario repetition=$repetition sessions=$sessions"
  start_capacity_stack "gizclaw-capacity-$project_slug" "$current_env" "$topology_flag"

  set -a
  # shellcheck disable=SC1090
  source "$current_env"
  set +a

  verify_capacity_service_image
  wait_capacity_post_start_settle

  capacity_edge_endpoint="$(resolve_capacity_edge_endpoint edge "$GIZCLAW_E2E_EDGE_ENDPOINT")"
  capacity_edge2_endpoint="$(resolve_capacity_edge_endpoint edge2 "$GIZCLAW_E2E_EDGE2_ENDPOINT")"
  coturn_a_container_id="$(resolve_compose_container_id coturn-a)"
  coturn_b_container_id="$(resolve_compose_container_id coturn-b)"
  read -r before_a_alloc before_a_recv before_a_sent before_b_alloc before_b_recv before_b_sent \
    < <(wait_coturn_allocation_count "$expected_allocations")

  coturn_live_artifact="${artifact%.json}-coturn-live.ndjson"
  coturn_monitor_stop="/tmp/gizclaw-${project_slug}-coturn-monitor.stop"
  (
    trap - EXIT
    monitor_coturn_allocations "$expected_allocations" "$coturn_live_artifact" "$coturn_monitor_stop"
  ) &
  coturn_monitor_pid="$!"
  wait_coturn_monitor_ready "$coturn_live_artifact"

  echo "==> run extended capacity workload: scenario=$scenario repetition=$repetition"
  # Leave reliable SCTP most of the 30-second round to recover while keeping
  # a two-second margin for artifact aggregation and the round deadline.
  set +e
  (cd "$repo_root" && exec env GOGC="$gateway_gogc" GOMAXPROCS="$gateway_gomaxprocs" "$gateway_bin" \
    -edges "$capacity_edge_endpoint,$capacity_edge2_endpoint" \
    -signaling-base-from-edge \
    -sessions "$sessions" \
    -ramp "$ramp" \
    -duration "$hold" \
    -ping-interval 30s \
    -dial-timeout "$gateway_dial_timeout" \
    -ping-timeout "$gateway_ping_timeout" \
    -speed-bytes "$gateway_speed_bytes" \
    -speed-baseline-bytes "$gateway_speed_baseline_bytes" \
    -speed-timeout "$gateway_speed_timeout" \
    -min-speed-aggregate-ratio "$gateway_min_speed_aggregate_ratio" \
    -min-upload-aggregate-mbps "$gateway_min_upload_aggregate_mbps" \
    -min-download-aggregate-mbps "$gateway_min_download_aggregate_mbps" \
    -min-final-speed-retention "$gateway_min_final_speed_retention" \
    -min-establishment-rate "$gateway_min_establishment_rate" \
    -max-dial-p95 "$gateway_max_dial_p95" \
    -max-dial-p99 "$gateway_max_dial_p99" \
    -concurrency "$gateway_concurrency" \
    -max-establishment-failures 0 \
    -max-ping-failures 0 \
    -max-ping-round-duration 30s \
    -require-balanced-edges \
    -max-sessions-per-edge "$max_sessions_per_edge" \
    -required-upstreams-per-edge "$gateway_required_upstreams_per_edge" \
    -max-upstreams-per-edge "$max_upstreams_per_edge" \
    -max-sessions-per-upstream "$max_sessions_per_upstream" \
    -upstream-path "$gateway_upstream_path" \
    -opus-packets 50 \
    -opus-packet-bytes 3 \
    -opus-interval 20ms \
    -require-role-resources \
    -docker-project "$GIZCLAW_E2E_DOCKER_PROJECT" \
    -docker-compose-file "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE" \
    -scenario "$scenario" \
    -repetition "$repetition" \
    -soak="$soak" \
    -cleanup-timeout "$gateway_cleanup_timeout" \
    -artifact "$artifact") &
  gateway_workload_pid="$!"
  monitor_status=0
  while kill -0 "$gateway_workload_pid" 2>/dev/null; do
    if ! kill -0 "$coturn_monitor_pid" 2>/dev/null; then
      wait "$coturn_monitor_pid"
      monitor_status="$?"
      coturn_monitor_pid=""
      if ((monitor_status == 0)); then
        echo "Coturn live-allocation monitoring stopped before the workload" >&2
        monitor_status=1
      fi
      echo "Coturn live-allocation monitoring failed; canceling scenario=$scenario repetition=$repetition" >&2
      kill -TERM "$gateway_workload_pid" 2>/dev/null || true
      break
    fi
    sleep 0.1
  done
  wait "$gateway_workload_pid"
  workload_status="$?"
  gateway_workload_pid=""
  if [[ -n "$coturn_monitor_pid" ]]; then
    stop_coturn_monitor
    monitor_status="$?"
  fi
  set -e
  if ((workload_status != 0)); then
    return "$workload_status"
  fi
  if ((monitor_status != 0)); then
    echo "Coturn live-allocation monitoring failed for scenario=$scenario repetition=$repetition" >&2
    return "$monitor_status"
  fi
  validate_coturn_live_samples "$expected_allocations" "$coturn_live_artifact"

  read -r after_a_alloc after_a_recv after_a_sent < <(read_coturn_metrics coturn-a)
  read -r after_b_alloc after_b_recv after_b_sent < <(read_coturn_metrics coturn-b)
  if [[ "$(numeric_sum "$after_a_alloc" "$after_b_alloc")" != "$expected_allocations" ]]; then
    echo "capacity workload changed the expected $expected_allocations-allocation upstream pool" >&2
    return 1
  fi
  edge_id="$(docker ps -q --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" --filter "label=com.docker.compose.service=edge")"
  edge2_id="$(docker ps -q --filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" --filter "label=com.docker.compose.service=edge2")"
  edge_log="${artifact%.json}-edge.log"
  edge2_log="${artifact%.json}-edge2.log"
  path_artifact="${artifact%.json}-path.json"
  (
    trap 'rm -f "$edge_log" "$edge2_log"' EXIT
    docker exec "$edge_id" cat /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log >"$edge_log"
    docker exec "$edge2_id" cat /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log >"$edge2_log"
    "$gateway_bin" \
      -collect-path-evidence \
      -upstream-path "$gateway_upstream_path" \
      -ice-logs "edge=$edge_log,edge2=$edge2_log" \
      -artifact "$path_artifact"
  )
  # The production Gateway may use its configured 30-second drain before it
  # closes the physical upstream pool. Compose's 10-second default can kill an
  # otherwise healthy Edge before that close reaches Coturn.
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY" \
    stop -t 45 edge edge2 >/dev/null
  wait_coturn_allocations_zero
  read -r cleanup_a_alloc cleanup_a_recv cleanup_a_sent < <(read_coturn_metrics coturn-a)
  read -r cleanup_b_alloc cleanup_b_recv cleanup_b_sent < <(read_coturn_metrics coturn-b)
  if [[ "$gateway_upstream_path" == "relay" ]]; then
    if ! numeric_greater "$(numeric_sum "$cleanup_a_recv" "$cleanup_b_recv")" "$(numeric_sum "$before_a_recv" "$before_b_recv")" ||
      ! numeric_greater "$(numeric_sum "$cleanup_a_sent" "$cleanup_b_sent")" "$(numeric_sum "$before_a_sent" "$before_b_sent")"; then
      echo "capacity workload did not increase both Coturn finished-session byte counters" >&2
      return 1
    fi
  elif [[ "$(numeric_sum "$cleanup_a_recv" "$cleanup_b_recv")" != "$(numeric_sum "$before_a_recv" "$before_b_recv")" ||
    "$(numeric_sum "$cleanup_a_sent" "$cleanup_b_sent")" != "$(numeric_sum "$before_a_sent" "$before_b_sent")" ]]; then
    echo "direct capacity workload unexpectedly sent traffic through Coturn" >&2
    return 1
  fi
  coturn_artifact="${artifact%.json}-coturn.json"
  jq -n \
    --arg image "coturn/coturn:4.7.0-r0@sha256:99bf5bf6ab1c119862d0c3d2dfb2bbf805a86a131492cab18c148be64ae7d978" \
    --arg version "4.7.0" \
    --arg upstream_path "$gateway_upstream_path" \
    --argjson before_a_alloc "$before_a_alloc" --argjson before_a_recv "$before_a_recv" --argjson before_a_sent "$before_a_sent" \
    --argjson before_b_alloc "$before_b_alloc" --argjson before_b_recv "$before_b_recv" --argjson before_b_sent "$before_b_sent" \
    --argjson after_a_alloc "$after_a_alloc" --argjson after_a_recv "$after_a_recv" --argjson after_a_sent "$after_a_sent" \
    --argjson after_b_alloc "$after_b_alloc" --argjson after_b_recv "$after_b_recv" --argjson after_b_sent "$after_b_sent" \
    --argjson cleanup_a_alloc "$cleanup_a_alloc" --argjson cleanup_a_recv "$cleanup_a_recv" --argjson cleanup_a_sent "$cleanup_a_sent" \
    --argjson cleanup_b_alloc "$cleanup_b_alloc" --argjson cleanup_b_recv "$cleanup_b_recv" --argjson cleanup_b_sent "$cleanup_b_sent" \
    --slurpfile live_samples "$coturn_live_artifact" \
    '{
      schema_version: 2,
      upstream_path: $upstream_path,
      passed: true,
      image: $image,
      version: $version,
      expected_gateway_allocations_per_edge: 4,
      expected_control_allocations_per_edge: 1,
      expected_total_allocations_per_edge: 5,
      maximum_live_sample_gap_seconds: 2.1,
      live_before: {
        coturn_a: {allocations: $before_a_alloc, received_bytes: $before_a_recv, sent_bytes: $before_a_sent},
        coturn_b: {allocations: $before_b_alloc, received_bytes: $before_b_recv, sent_bytes: $before_b_sent}
      },
      after_workload: {
        coturn_a: {allocations: $after_a_alloc, received_bytes: $after_a_recv, sent_bytes: $after_a_sent},
        coturn_b: {allocations: $after_b_alloc, received_bytes: $after_b_recv, sent_bytes: $after_b_sent}
      },
      live_samples: $live_samples,
      cleanup: {
        coturn_a: {allocations: $cleanup_a_alloc, received_bytes: $cleanup_a_recv, sent_bytes: $cleanup_a_sent},
        coturn_b: {allocations: $cleanup_b_alloc, received_bytes: $cleanup_b_recv, sent_bytes: $cleanup_b_sent},
        allocations_returned_to_zero_within_seconds: 15
      },
      traffic_delta: {
        received_bytes: (($cleanup_a_recv + $cleanup_b_recv) - ($before_a_recv + $before_b_recv)),
        sent_bytes: (($cleanup_a_sent + $cleanup_b_sent) - ($before_a_sent + $before_b_sent))
      }
    }' >"$coturn_artifact"
  rm -f "$coturn_live_artifact"

  echo "==> tear down fresh capacity stack: scenario=$scenario repetition=$repetition"
  cleanup_current
}

run_extended_matrix() {
  for repetition in 1 2 3; do
    run_case sessions-100 100 30s 5m "$repetition" false
  done
  for repetition in 1 2 3; do
    run_case sessions-500 500 2m30s 5m "$repetition" false
  done
  for repetition in 1 2 3; do
    run_case sessions-1000 1000 5m 5m "$repetition" false
  done
  run_case soak-1000 1000 5m 60m 1 true

  echo "==> analyze extended capacity artifacts"
  (cd "$repo_root" && "$gateway_bin" \
    -analyze-dir "$runs_dir" \
    -artifact "$artifact_set/projection.json")

  echo "==> extended gateway-capacity artifact set: $artifact_set"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  run_extended_matrix
fi
