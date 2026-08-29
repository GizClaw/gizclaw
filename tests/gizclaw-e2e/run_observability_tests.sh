#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
credential_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"

cleanup() {
  local status="$?"
  if ((status != 0)) && [[ -f "$script_dir/testdata/docker/current.env" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$script_dir/testdata/docker/current.env"
    set +a
    local -a compose_args=(-p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE")
    if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
      compose_args+=(-f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")
    fi
    docker compose "${compose_args[@]}" logs --tail=200 server edge metrics-sink >&2 || true
  fi
  bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1 || true
}
trap cleanup EXIT

export GIZCLAW_E2E_CREDENTIAL_FILE="$credential_file"
mkdir -p "$script_dir/testdata/bin"
(cd "$repo_root" && go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw)
bash "$setup_dir/docker-compose-up.sh" --observability
set -a
# shellcheck disable=SC1091
source "$script_dir/testdata/docker/current.env"
set +a
export GIZCLAW_TEST_ENDPOINT="$GIZCLAW_E2E_EDGE_ENDPOINT"
export GIZCLAW_E2E_OBSERVABILITY=1

(cd "$repo_root" && "$script_dir/testdata/bin/gizclaw" test run \
  "$script_dir/giztest/eino-memory-assistant.text-roundtrip.giztest.yaml" \
  --parallel 1 --output "$script_dir/testdata/giztest-observability-report.json")

# Recorders flush every ten seconds; wait for one complete interval before querying.
sleep 12
(cd "$repo_root" && go test -count=1 -v -tags gizclaw_e2e \
  -run '^TestAdminConversationAuditLogs$' ./tests/gizclaw-e2e/go/admin)

compose_args=(-p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$GIZCLAW_E2E_DOCKER_COMPOSE_FILE")
if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
  compose_args+=(-f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")
fi
docker compose "${compose_args[@]}" exec -T metrics-sink curl -fsS http://127.0.0.1:9090/dump |
  python3 -c '
import json, sys
samples = json.load(sys.stdin)
by_name = {}
for sample in samples:
    by_name.setdefault(sample["name"], []).append(sample)
required = {
    "giz_webrtc_signaling_requests_total",
    "giz_webrtc_connections_total",
    "giz_webrtc_service_requests_total",
    "giz_edge_webrtc_requests_total",
    "giz_edge_webrtc_session_establishments_total",
    "giz_edge_webrtc_bridges_total",
}
missing = sorted(required - by_name.keys())
if missing:
    raise SystemExit("missing required metrics: " + ", ".join(missing))
roles = {sample["labels"].get("node_role") for name in (
    "giz_webrtc_signaling_requests_total", "giz_webrtc_connections_total"
) for sample in by_name.get(name, [])}
if not {"application", "edge"}.issubset(roles):
    raise SystemExit(f"missing WebRTC node_role coverage: got {sorted(str(role) for role in roles)}")
print(f"observability metrics accepted: samples={len(samples)} required={len(required)} node_roles={sorted(roles)}")
'
