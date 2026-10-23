#!/usr/bin/env bash
# Real Lite native PTT with pre/post text dependencies on local Docker Server/Edge.
set -euo pipefail
umask 077
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/native-audio-functions-XXXXXX")"
gizclaw_binary="$run_dir/gizclaw"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-native-audio-functions-$$"
export GIZCLAW_E2E_DOCKER_ENV="$run_dir/docker.env"
project="$GIZCLAW_E2E_DOCKER_PROJECT"
compose=(docker compose -p "$project" -f "$script_dir/docker/docker-compose.yaml")
stack_started=0

# shellcheck source=setup/credentials.sh
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"
export GIZCLAW_E2E_CREDENTIAL_FILE="$env_file"
# Load only this lane's real providers and resources, not unrelated fixtures.
export GIZCLAW_E2E_RESOURCE_PATHS="00-credentials/01-volc.yaml 01-tenants/01-volc.yaml 01-tenants/02-volc-ark.yaml 03-models/01-volc-tts.yaml 03-models/02-volc-asr.yaml 03-models/11-doubao-lite-audio-chat.yaml 04-workflows/46-eino-native-audio-functions.yaml 09-giztest/04-native-audio-functions.yaml"

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
"$gizclaw_binary" admin validate -f "$script_dir/testdata/resources/04-workflows/46-eino-native-audio-functions.yaml"
"$gizclaw_binary" test validate -f "$script_dir/giztest/eino-native-audio-functions.push-to-talk-transcript.giztest.yaml"
"$gizclaw_binary" test validate -f "$script_dir/giztest/eino-native-audio-functions.push-to-talk-first-response.giztest.yaml"
echo "==> start isolated local Docker stack project=$project"
bash "$setup_dir/docker-compose-up.sh" > "$run_dir/stack.log" 2>&1
stack_started=1
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a

python3 - "$repo_root" "$run_dir/manifest.json" "$gizclaw_binary" "$project" <<'PY'
import hashlib
import json
from pathlib import Path
import subprocess
import sys
root, output, binary = map(Path, sys.argv[1:4])
paths = ["pkgs/genx/agentkit/audiodock/dock.go", "pkgs/genx/agentkit/audiodock/input_text.go", "pkgs/genx/transformers/eino/builder.go", "pkgs/genx/transformers/eino/runtime_state.go", "pkgs/genx/transformers/eino/transformer.go", "pkgs/gizclaw/services/ai/workflow/agents/eino/factory.go", "tests/gizclaw-e2e/run_native_audio_functions_tests.sh", "tests/gizclaw-e2e/giztest/eino-native-audio-functions.push-to-talk-transcript.giztest.yaml", "tests/gizclaw-e2e/testdata/resources/04-workflows/46-eino-native-audio-functions.yaml", "tests/gizclaw-e2e/testdata/resources/03-models/11-doubao-lite-audio-chat.yaml", "tests/gizclaw-e2e/testdata/resources/09-giztest/01-runtime-profile.yaml"]
paths += ["tests/gizclaw-e2e/giztest/eino-native-audio-functions.push-to-talk-first-response.giztest.yaml", "tests/gizclaw-e2e/testdata/resources/09-giztest/04-native-audio-functions.yaml"]
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "dirty": bool(subprocess.check_output(["git", "-C", str(root), "status", "--porcelain"], text=True).strip()),
    "project": sys.argv[4], "model": "doubao-seed-2-1-lite-260915", "thinking": "disabled",
    "cli_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
    "sha256": {name: hashlib.sha256((root / name).read_bytes()).hexdigest() for name in paths},
}, indent=2) + "\n")
PY
"${compose[@]}" images --format json > "$run_dir/docker-images.json"

for route in server edge; do
  if [[ "$route" == server ]]; then
    export GIZCLAW_TEST_ENDPOINT="$GIZCLAW_E2E_SERVER_ENDPOINT"
  else
    export GIZCLAW_TEST_ENDPOINT="$GIZCLAW_E2E_EDGE_ENDPOINT"
  fi
  for mode in post pre; do
    export GIZCLAW_NATIVE_AUDIO_WORKFLOW=eino-native-audio-functions
    if [[ "$mode" == pre ]]; then
      export GIZCLAW_NATIVE_AUDIO_WORKFLOW=eino-native-audio-pre-text
    fi
    for kind in first-response transcript; do
      echo "==> native PTT: route=$route text_dependency=$mode kind=$kind"
      "$gizclaw_binary" test run --parallel 1 --evidence full \
        --output "$run_dir/$route-$mode-$kind" --report "$run_dir/$route-$mode-$kind.json" \
        "$script_dir/giztest/eino-native-audio-functions.push-to-talk-$kind.giztest.yaml" \
        > "$run_dir/$route-$mode-$kind.log" 2>&1
    done
  done
done
echo "==> all eight native-audio Docker Giztests passed"
