#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
artifact_root="$script_dir/testdata/gateway-capacity-extended"
artifact_base="${GIZCLAW_E2E_GATEWAY_EXTENDED_ARTIFACT_DIR:-$artifact_root}"
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
current_env=""
runtime_state=""
gateway_gomaxprocs="${GIZCLAW_E2E_GATEWAY_GOMAXPROCS:-8}"
gateway_dial_timeout="${GIZCLAW_E2E_GATEWAY_DIAL_TIMEOUT:-20s}"
gateway_ping_timeout="${GIZCLAW_E2E_GATEWAY_PING_TIMEOUT:-28s}"
gateway_speed_bytes="${GIZCLAW_E2E_GATEWAY_SPEED_BYTES:-0}"
gateway_speed_baseline_bytes="${GIZCLAW_E2E_GATEWAY_SPEED_BASELINE_BYTES:-0}"
gateway_speed_timeout="${GIZCLAW_E2E_GATEWAY_SPEED_TIMEOUT:-2m}"
gateway_min_speed_aggregate_ratio="${GIZCLAW_E2E_GATEWAY_MIN_SPEED_AGGREGATE_RATIO:-0}"
gateway_min_upload_aggregate_mbps="${GIZCLAW_E2E_GATEWAY_MIN_UPLOAD_AGGREGATE_MBPS:-0}"
gateway_min_download_aggregate_mbps="${GIZCLAW_E2E_GATEWAY_MIN_DOWNLOAD_AGGREGATE_MBPS:-0}"
gateway_min_establishment_rate="${GIZCLAW_E2E_GATEWAY_MIN_ESTABLISHMENT_RATE:-0}"
gateway_max_dial_p95="${GIZCLAW_E2E_GATEWAY_MAX_DIAL_P95:-0}"
gateway_max_dial_p99="${GIZCLAW_E2E_GATEWAY_MAX_DIAL_P99:-0}"
gateway_concurrency="${GIZCLAW_E2E_GATEWAY_CONCURRENCY:-512}"
gateway_required_upstreams_per_edge="${GIZCLAW_E2E_GATEWAY_REQUIRED_UPSTREAMS_PER_EDGE:-4}"
gateway_upstream_path="${GIZCLAW_E2E_GATEWAY_UPSTREAM_PATH:-relay}"
gateway_prebuilt="${GIZCLAW_E2E_GATEWAY_PREBUILT:-0}"
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
if ! command -v jq >/dev/null 2>&1; then
  echo "extended capacity requires jq to write Coturn evidence" >&2
  exit 2
fi

mkdir -p "$artifact_root" "$script_dir/testdata/docker" "$script_dir/testdata/bin"
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
  if [[ -z "$current_env" || ! -f "$current_env" ]]; then
    return 0
  fi
  GIZCLAW_E2E_DOCKER_ENV="$current_env" bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || return 1
  rm -f "$current_env"
  current_env=""
}

cleanup_on_exit() {
  local status="$?"
  if ! cleanup_current; then
    echo "failed to clean the active gateway-capacity Docker project; env=$current_env" >&2
    status=1
  fi
  rmdir "$runtime_state" >/dev/null 2>&1 || true
  exit "$status"
}
trap cleanup_on_exit EXIT

read_gateway_limit() {
  local key="$1"
  awk -v key="$key:" '$1 == key { print $2; found = 1; exit } END { if (!found) exit 1 }' \
    "$script_dir/testdata/edge-workspace/config.yaml.template"
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
  local output
  output="$(docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY" \
    exec -T "$service" bash -lc '
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

numeric_sum() {
  awk -v left="$1" -v right="$2" 'BEGIN { printf "%.0f\n", left + right }'
}

numeric_greater() {
  awk -v value="$1" -v baseline="$2" 'BEGIN { exit !(value > baseline) }'
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
  if [[ ! -x "$script_dir/testdata/bin/gizclaw" || ! -x "$gateway_bin" ]]; then
    echo "prebuilt capacity binaries are missing" >&2
    exit 2
  fi
else
  echo "==> build host e2e CLI and extended gateway-capacity runner"
  (cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
  (cd "$repo_root" && go build -o "$gateway_bin" ./tests/gizclaw-e2e/gateway-capacity)
fi

run_case() {
  local scenario="$1"
  local sessions="$2"
  local ramp="$3"
  local hold="$4"
  local repetition="$5"
  local soak="$6"
  local project_slug artifact coturn_artifact path_artifact capacity_edge_endpoint capacity_edge2_endpoint
  local topology_flag expected_allocations edge_log edge2_log edge_id edge2_id
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
  GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-capacity-$project_slug" \
    GIZCLAW_E2E_DOCKER_ENV="$current_env" \
    bash "$setup_dir/docker-compose-up.sh" "$topology_flag"

  set -a
  # shellcheck disable=SC1090
  source "$current_env"
  set +a

  capacity_edge_endpoint="$(resolve_capacity_edge_endpoint edge "$GIZCLAW_E2E_EDGE_ENDPOINT")"
  capacity_edge2_endpoint="$(resolve_capacity_edge_endpoint edge2 "$GIZCLAW_E2E_EDGE2_ENDPOINT")"
  read -r before_a_alloc before_a_recv before_a_sent before_b_alloc before_b_recv before_b_sent \
    < <(wait_coturn_allocation_count "$expected_allocations")

  echo "==> run extended capacity workload: scenario=$scenario repetition=$repetition"
  # Leave reliable SCTP most of the 30-second round to recover while keeping
  # a two-second margin for artifact aggregation and the round deadline.
  (cd "$repo_root" && GOMAXPROCS="$gateway_gomaxprocs" "$gateway_bin" \
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
    -artifact "$artifact")

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
  docker exec "$edge_id" cat /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log >"$edge_log"
  docker exec "$edge2_id" cat /src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log >"$edge2_log"
  path_artifact="${artifact%.json}-path.json"
  "$gateway_bin" \
    -collect-path-evidence \
    -upstream-path "$gateway_upstream_path" \
    -ice-logs "edge=$edge_log,edge2=$edge2_log" \
    -artifact "$path_artifact"
  rm -f "$edge_log" "$edge2_log"
  docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE" \
    -f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY" \
    stop edge edge2 >/dev/null
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
    '{
      schema_version: 1,
      upstream_path: $upstream_path,
      passed: true,
      image: $image,
      version: $version,
      expected_gateway_allocations_per_edge: 4,
      expected_control_allocations_per_edge: 1,
      expected_total_allocations_per_edge: 5,
      live_before: {
        coturn_a: {allocations: $before_a_alloc, received_bytes: $before_a_recv, sent_bytes: $before_a_sent},
        coturn_b: {allocations: $before_b_alloc, received_bytes: $before_b_recv, sent_bytes: $before_b_sent}
      },
      after_workload: {
        coturn_a: {allocations: $after_a_alloc, received_bytes: $after_a_recv, sent_bytes: $after_a_sent},
        coturn_b: {allocations: $after_b_alloc, received_bytes: $after_b_recv, sent_bytes: $after_b_sent}
      },
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
