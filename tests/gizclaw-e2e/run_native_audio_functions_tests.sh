#!/usr/bin/env bash
# Real Lite native PTT with text dependencies or schema control on Docker Server/Edge.
set -euo pipefail
umask 077
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
lane="${1:-functions}"
case "$lane" in
  functions)
    modes=(post pre)
    workflow_file=46-eino-native-audio-functions.yaml
    profile_file=04-native-audio-functions.yaml
    scenario_dir=native-audio-functions
    extra_model=""
    ;;
  structured)
    modes=(structured)
    workflow_file=47-eino-native-structured-control.yaml
    profile_file=05-native-structured-control.yaml
    scenario_dir=native-structured-control
    extra_model="03-models/12-doubao-lite-structured-control.yaml"
    ;;
  *) echo "Expected functions or structured lane" >&2; exit 2 ;;
esac
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/native-audio-$lane-XXXXXX")"
gizclaw_binary="$run_dir/gizclaw"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-native-audio-$lane-$$"
export GIZCLAW_E2E_DOCKER_ENV="$run_dir/docker.env"
project="$GIZCLAW_E2E_DOCKER_PROJECT"
compose=(docker compose -p "$project" -f "$script_dir/docker/docker-compose.yaml")
stack_started=0

# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"
export GIZCLAW_E2E_CREDENTIAL_FILE="$env_file"
# Load only this lane's real providers and resources, not unrelated fixtures.
export GIZCLAW_E2E_RESOURCE_PATHS="00-credentials/01-volc.yaml 01-tenants/01-volc.yaml 01-tenants/02-volc-ark.yaml 02-voices/01-chat.yaml 03-models/01-volc-tts.yaml 03-models/02-volc-asr.yaml 03-models/11-doubao-lite-audio-chat.yaml $extra_model 04-workflows/$workflow_file 09-giztest/$profile_file"

cleanup() {
  local status=$?
  trap - EXIT
  if [[ "$stack_started" == 1 || -f "$GIZCLAW_E2E_DOCKER_ENV" ]]; then
    if [[ -f "$GIZCLAW_E2E_DOCKER_ENV" ]]; then
      set -a
      # shellcheck disable=SC1090
      source "$GIZCLAW_E2E_DOCKER_ENV"
      set +a
    fi
    "${compose[@]}" logs --no-color server edge edge2 2>&1 |
      python3 "$setup_dir/redact_diagnostics.py" > "$run_dir/containers.log" || true
    for service in server edge edge2; do
      log_path="/src/tests/gizclaw-e2e/testdata/edge-workspace/gizclaw-edge.log"
      if [[ "$service" == server ]]; then
        log_path="/src/tests/gizclaw-e2e/testdata/server-workspace/gizclaw-server.log"
      fi
      "${compose[@]}" exec -T "$service" cat "$log_path" 2>&1 |
        python3 "$setup_dir/redact_diagnostics.py" > "$run_dir/$service-runtime.log" || true
    done
    if ! bash "$setup_dir/docker-compose-down.sh" > "$run_dir/cleanup.log" 2>&1; then
      status=1
    fi
  fi
  rm -f "$GIZCLAW_E2E_DOCKER_ENV" "$gizclaw_binary"
  echo "Native audio Docker E2E evidence: $run_dir"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

echo "==> build task-local CLI"
(cd "$repo_root" && npm ci && npm run build:console && go build -o "$gizclaw_binary" ./cmd/gizclaw) > "$run_dir/build.log" 2>&1
"$gizclaw_binary" admin validate -f "$script_dir/testdata/resources/04-workflows/$workflow_file"
"$gizclaw_binary" test validate -f "$script_dir/testdata/$scenario_dir/roundtrip.giztest.yaml"
"$gizclaw_binary" test validate -f "$script_dir/testdata/$scenario_dir/first-response.giztest.yaml"
echo "==> start isolated local Docker stack project=$project"
bash "$setup_dir/docker-compose-up.sh" > "$run_dir/stack.log" 2>&1
stack_started=1
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a
export GIZCLAW_TEST_REGISTRATION_TOKEN="gizclaw-e2e-$scenario_dir"

python3 - "$repo_root" "$run_dir/manifest.json" "$gizclaw_binary" "$project" <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess
import sys
root, output, binary = map(Path, sys.argv[1:4])
paths = ["pkgs/genx/agentkit/audiodock/dock.go", "pkgs/genx/agentkit/audiodock/input_text.go", "pkgs/genx/transformers/eino/script.go", "pkgs/genx/transformers/eino/script_input_budget_test.go", "pkgs/genx/transformers/eino/native_script_budget_test.go", "pkgs/genx/transformers/eino/builder.go", "pkgs/genx/transformers/eino/runtime_state.go", "pkgs/genx/transformers/eino/transformer.go", "pkgs/gizclaw/services/ai/workflow/agents/eino/factory.go", "tests/gizclaw-e2e/run_native_audio_functions_tests.sh", "tests/gizclaw-e2e/testdata/native-audio-functions/roundtrip.giztest.yaml", "tests/gizclaw-e2e/testdata/resources/04-workflows/46-eino-native-audio-functions.yaml", "tests/gizclaw-e2e/testdata/resources/03-models/11-doubao-lite-audio-chat.yaml", "tests/gizclaw-e2e/testdata/resources/09-giztest/01-runtime-profile.yaml"]
paths += ["pkgs/gizclaw/services/runtime/agenthost/history.go", "pkgs/gizclaw/services/runtime/agenthost/history_persistence.go", "pkgs/gizclaw/services/runtime/agenthost/history_output_terminal.go", "pkgs/gizclaw/services/runtime/agenthost/history_output_latency_test.go", "pkgs/gizclaw/services/runtime/agenthost/history_barrier.go", "cmd/internal/commands/giztest/peer_stream.go", "cmd/internal/commands/giztest/peer_stream_history.go", "cmd/internal/commands/giztest/peer_stream_history_test.go", "cmd/internal/commands/giztest/peer_stream_test.go", "cmd/internal/commands/giztest/peer_stream_native_probe_test.go", "tests/gizclaw-e2e/testdata/native-audio-functions/first-response.giztest.yaml", "tests/gizclaw-e2e/testdata/resources/09-giztest/04-native-audio-functions.yaml"]
paths += ["api/http/shared/workflows/eino.json", "pkgs/genx/transformers/eino/nodes.go", "pkgs/genx/transformers/eino/components.go", "pkgs/genx/transformers/eino/config.go", "pkgs/genx/transformers/eino/structured_output.go", "pkgs/gizclaw/services/ai/workflow/einoconfig/config.go", "pkgs/gizclaw/api/apitypes/generated.go", "tests/gizclaw-e2e/run_native_structured_output_tests.sh", "tests/gizclaw-e2e/testdata/native-structured-control/first-response.giztest.yaml", "tests/gizclaw-e2e/testdata/native-structured-control/roundtrip.giztest.yaml", "tests/gizclaw-e2e/testdata/resources/04-workflows/47-eino-native-structured-control.yaml", "tests/gizclaw-e2e/testdata/resources/03-models/12-doubao-lite-structured-control.yaml", "tests/gizclaw-e2e/testdata/resources/09-giztest/05-native-structured-control.yaml"]
changed = subprocess.check_output(["git", "-C", str(root), "ls-files", "-m", "--others", "--exclude-standard"], text=True).splitlines()
paths = sorted(set(paths + [name for name in changed if (root / name).is_file()]))
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "dirty": bool(subprocess.check_output(["git", "-C", str(root), "status", "--porcelain"], text=True).strip()),
    "project": sys.argv[4], "model": "doubao-seed-2-1-lite-260915", "thinking": "disabled",
    "cli_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
    "sha256": {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in paths},
}, indent=2) + "\n")
PY
"${compose[@]}" images --format json > "$run_dir/docker-images.json"

failed_cases=()
for route in server edge; do
  if [[ "$route" == server ]]; then
    export GIZCLAW_TEST_ENDPOINT="$GIZCLAW_E2E_SERVER_ENDPOINT"
  else
    export GIZCLAW_TEST_ENDPOINT="$GIZCLAW_E2E_EDGE_ENDPOINT"
  fi
  for mode in "${modes[@]}"; do
    export GIZCLAW_NATIVE_AUDIO_WORKFLOW=eino-native-audio-functions
    if [[ "$mode" == pre ]]; then
      export GIZCLAW_NATIVE_AUDIO_WORKFLOW=eino-native-audio-pre-text
    elif [[ "$mode" == structured ]]; then
      export GIZCLAW_NATIVE_AUDIO_WORKFLOW=eino-native-structured-control
    fi
    for kind in first-response roundtrip; do
      echo "==> native PTT: route=$route text_dependency=$mode kind=$kind"
      if ! "$gizclaw_binary" test run --parallel 1 --evidence full \
        --output "$run_dir/$route-$mode-$kind" --report "$run_dir/$route-$mode-$kind.json" \
        "$script_dir/testdata/$scenario_dir/$kind.giztest.yaml" \
        > "$run_dir/$route-$mode-$kind.log" 2>&1; then
        failed_cases+=("$route-$mode-$kind")
      fi
    done
  done
done
if [[ "${#failed_cases[@]}" != 0 ]]; then
  printf 'Native audio Docker Giztests failed: %s\n' "${failed_cases[*]}" >&2
  exit 1
fi
echo "==> all $((4 * ${#modes[@]})) native-audio Docker Giztests passed"
