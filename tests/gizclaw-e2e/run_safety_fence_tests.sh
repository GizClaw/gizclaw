#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
credential_file="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-safety-fence-${project_user:-user}-$$"
export GIZCLAW_E2E_DOCKER_ENV="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT.env"
export GIZCLAW_E2E_CREDENTIAL_FILE="$credential_file"
binary="$script_dir/testdata/bin/gizclaw-$GIZCLAW_E2E_DOCKER_PROJECT"
report_dir="${GIZCLAW_SAFETY_FENCE_REPORT_DIR:-$script_dir/.testbench/$GIZCLAW_E2E_DOCKER_PROJECT}"

# This directory is deliberately outside the standard [0-9][0-9]-* fixture
# discovery. Its Profile has only the resources needed by these nine scenarios.
resource_paths=(
	00-credentials/01-volc.yaml
	00-credentials/05-qwen-dashscope.yaml
	01-tenants/01-volc.yaml
	01-tenants/02-volc-ark.yaml
	01-tenants/05-qwen-dashscope.yaml
	02-voices/00-doubao-realtime-vivi.yaml
	02-voices/01-chat.yaml
	03-models/01-volc-tts.yaml
	03-models/02-volc-asr.yaml
	03-models/03-doubao-realtime.yaml
	03-models/04-doubao-lite-chat.yaml
	03-models/09-qwen-realtime.yaml
	03-models/10-doubao-realtime-duplex.yaml
	04-workflows/41-safety-fence.yaml
	safety-fence/profile.yaml
	safety-fence/token.yaml
)
export GIZCLAW_E2E_RESOURCE_PATHS="${resource_paths[*]}"

cleanup() {
	local status=$?
	if [[ -f "$GIZCLAW_E2E_DOCKER_ENV" || "$stack_started" == "1" ]]; then
		if ! bash "$setup_dir/docker-compose-down.sh" && ((status == 0)); then
			status=1
		fi
	fi
	rm -f "$GIZCLAW_E2E_DOCKER_ENV" "$binary"
	echo "Safety fence Giztest reports: $report_dir"
	exit "$status"
}
stack_started=0
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$credential_file"
mkdir -p "$script_dir/testdata/bin" "$script_dir/testdata/docker" "$report_dir"

(cd "$repo_root" && npm ci && npm run build:console && go build -o "$binary" ./cmd/gizclaw)

echo "==> start isolated safety fence stack project=$GIZCLAW_E2E_DOCKER_PROJECT"
stack_started=1
bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a

"$binary" test run \
	"$script_dir/giztest/server.workspace.safety-fence.roundtrip.giztest.yaml" \
	"$script_dir/giztest/server.workspace.safety-fence.missing-profile.giztest.yaml" \
	--parallel 2 --output "$report_dir/rpc.json"

"$binary" test run "$script_dir"/giztest/safety-fence-*.giztest.yaml \
	--parallel 5 --output "$report_dir/provider.json"

"$binary" test run "$script_dir/giztest/server.device.runtime_profile.get.giztest.yaml" \
	--parallel 1 --output "$report_dir/http.json"

GIZCLAW_TEST_EDGE_A="$GIZCLAW_E2E_EDGE_ENDPOINT" \
	GIZCLAW_TEST_EDGE_B="$GIZCLAW_E2E_EDGE2_ENDPOINT" \
	GIZCLAW_TEST_REGISTRATION_TOKEN_A="$GIZCLAW_TEST_REGISTRATION_TOKEN" \
	GIZCLAW_TEST_REGISTRATION_TOKEN_B="$GIZCLAW_TEST_REGISTRATION_TOKEN" \
	"$binary" test run "$script_dir/giztest/sfu.workspace.switch.giztest.yaml" \
	--parallel 1 --output "$report_dir/sfu.json"
