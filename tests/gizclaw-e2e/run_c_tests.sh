#!/usr/bin/env bash
# Run the C SDK suites and shared Giztest scenarios against a TiKV Docker stack.
set -euo pipefail
umask 077
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
evidence="${GIZCLAW_E2E_C_EVIDENCE_DIR:-$repo_root/.testbench/c-e2e-$(date +%Y%m%d-%H%M%S)-$$}"
reuse_env=""
giztest_only=0
while (($# > 0)); do
  case "$1" in
    --reuse-env) reuse_env="${2:?--reuse-env requires a runtime env file}"; shift 2 ;;
    --giztest-only) giztest_only=1; shift ;;
    *) echo "usage: $0 [--reuse-env <runtime.env>] [--giztest-only]" >&2; exit 2 ;;
  esac
done
mkdir -p "$evidence"
evidence="$(cd "$evidence" && pwd)"
if [[ -e "$evidence/phases.log" ]]; then echo 'C evidence directory already contains a run' >&2; exit 2; fi
# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy
cd "$repo_root"
collector_pid=""
active_pid=""
owned_stack=0
failures=()
terminate_children() {
  local pid="$1" child
  while IFS= read -r child; do
    [[ -z "$child" ]] || terminate_children "$child"
  done < <(pgrep -P "$pid" 2>/dev/null || true)
  kill -TERM "$pid" 2>/dev/null || true
}
phase() { printf 'C_E2E_PHASE %s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" | tee -a "$evidence/phases.log"; }
run_logged() {
  local name="$1"
  shift
  phase "$name"
  local status=0
  ("$@" 2>&1 | python3 -B "$setup_dir/redact_diagnostics.py" | tee "$evidence/$name.log") &
  active_pid=$!
  wait "$active_pid" || status=$?
  active_pid=""
  phase "${name}_done_status_$status"
  return "$status"
}
cleanup() {
  local status=$?
  trap - EXIT
  set +e
  if [[ -n "$active_pid" ]]; then terminate_children "$active_pid"; fi
  touch "$evidence/stop-resources"
  if [[ -n "$collector_pid" ]]; then wait "$collector_pid"; fi
  local role path
  for role in server edge edge2; do
    [[ -n "${GIZCLAW_E2E_DOCKER_PROJECT:-}" ]] || break
    path=server-workspace/gizclaw-server.log
    if [[ "$role" != server ]]; then path=edge-workspace/gizclaw-edge.log; fi
    docker exec "${GIZCLAW_E2E_DOCKER_PROJECT}-$role-1" cat "/src/tests/gizclaw-e2e/testdata/$path" 2>&1 \
      | python3 -B "$setup_dir/redact_diagnostics.py" > "$evidence/$role.log"
  done
  if [[ "$owned_stack" == 1 ]]; then
    bash "$setup_dir/docker-compose-down.sh" --timeout 5 2>&1 \
      | python3 -B "$setup_dir/redact_diagnostics.py" > "$evidence/cleanup.log"
    if ((PIPESTATUS[0] != 0)); then status=1; fi
  fi
  phase "exit_status_$status"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 124' INT TERM
if [[ -n "$reuse_env" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$reuse_env"
  set +a
else
  export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-c-e2e-$$"
  export GIZCLAW_E2E_DOCKER_ENV="$evidence/docker.env"
  owned_stack=1
  run_logged docker-setup bash "$setup_dir/docker-compose-up.sh"
  set -a
  # shellcheck disable=SC1090
  source "$GIZCLAW_E2E_DOCKER_ENV"
  set +a
fi
: "${GIZCLAW_E2E_DOCKER_PROJECT:?runtime env must identify the Docker project}"
if [[ ! "$GIZCLAW_E2E_DOCKER_PROJECT" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then echo 'invalid Docker project' >&2; exit 2; fi
# Keep native includes current: Go's cache does not track SDK C files outside
# the bridge package. Rebuild each bridge once, then run the actual suites.
run_logged nanopb git submodule update --init --recursive -- third_party/nanopb/upstream
run_logged c-runner-build go build -a -o "$evidence/giztest-c" ./tests/gizclaw-e2e/cgo/giztest
if [[ "$giztest_only" != 1 ]]; then
  mkdir -p "$script_dir/testdata/bin"
  run_logged host-cli-build go build -o "$script_dir/testdata/bin/gizclaw" ./cmd/gizclaw
  run_logged c-bridge-build go test -a -tags gizclaw_e2e -run '^$' ./tests/gizclaw-e2e/cgo/internal
fi
python3 -B "$setup_dir/collect_c_resources.py" --project "$GIZCLAW_E2E_DOCKER_PROJECT" \
  --output "$evidence/resources" --stop-file "$evidence/stop-resources" &
collector_pid=$!
# Enable measurements on this fixture database; fsync and commit settings stay
# unchanged. The timings are read by the resource collector's pg_stat_wal.
run_logged postgres-timing docker exec "$GIZCLAW_E2E_DOCKER_PROJECT-postgres-1" \
  psql -U gizclaw_bootstrap -d postgres -At \
  -c 'ALTER SYSTEM SET track_io_timing=on' -c 'ALTER SYSTEM SET track_wal_io_timing=on' -c 'SELECT pg_reload_conf()'
if [[ "$giztest_only" != 1 ]]; then
  for package in rpc telemetry chat media social; do
    if run_logged "cgo-$package" go test -v -tags gizclaw_e2e -count=1 -timeout 10m "./tests/gizclaw-e2e/cgo/$package"; then :; else failures+=("cgo-$package"); fi
  done
fi
files=()
while IFS= read -r file; do files+=("$file"); done < <(
  find "$script_dir/giztest" -maxdepth 1 -type f -name '*.giztest.yaml' \
    ! -name 'failure-cleanup.giztest.yaml' ! -name 'server.monitor.*' ! -name 'sfu.*' \
    ! -name 'slow-tts.*' ! -name 'speaker-segments.*' -print | sort
)
parallel="${GIZCLAW_E2E_C_PARALLEL:-8}"
if [[ ! "$parallel" =~ ^[1-9][0-9]*$ ]]; then echo 'C parallel must be a positive integer' >&2; exit 2; fi
if run_logged c-giztest "$evidence/giztest-c" test run "${files[@]}" --parallel "$parallel" --output "$evidence/c-report.json"; then :; else failures+=(c-giztest); fi
python3 - "$evidence" <<'PY'
import hashlib, json, pathlib, subprocess, sys
folder=pathlib.Path(sys.argv[1])
receipt={"git_head":subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip(),"source_diff_sha256":hashlib.sha256(subprocess.check_output(["git","diff","HEAD"])).hexdigest(),"runner_sha256":hashlib.sha256((folder/"giztest-c").read_bytes()).hexdigest()}
(folder/"source.json").write_text(json.dumps(receipt,indent=2)+"\n")
PY
if ((${#failures[@]} > 0)); then printf 'C E2E failed phases: %s\n' "${failures[*]}" >&2; exit 1; fi
