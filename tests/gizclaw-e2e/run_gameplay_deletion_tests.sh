#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
cleanup_done=0
project_suffix="$(printf '%s-%s' "${USER:-user}" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-gameplay-delete-$project_suffix"
export GIZCLAW_E2E_DOCKER_ENV="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT.env"
export GIZCLAW_E2E_SYNC_VOLC_TENANT_ID=""

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
	local status="$?"
	if [[ "$cleanup_done" == "0" && -f "$GIZCLAW_E2E_DOCKER_ENV" ]]; then
		if ! bash "$setup_dir/docker-compose-down.sh" >/dev/null 2>&1; then
			status=1
		fi
	fi
	rm -f "$GIZCLAW_E2E_DOCKER_ENV"
	exit "$status"
}
trap cleanup EXIT

bash "$setup_dir/docker-compose-up.sh"
set -a
# shellcheck disable=SC1090
source "$GIZCLAW_E2E_DOCKER_ENV"
set +a

(cd "$repo_root" && go test -count=1 -v -tags gizclaw_e2e -timeout 5m \
	-run '^TestGameplayPetDeletionFinalizesThroughManagedProcessor$' \
	./tests/gizclaw-e2e/go/gameplay)

project="$GIZCLAW_E2E_DOCKER_PROJECT"
state_dir="$script_dir/testdata/docker/$project"
bash "$setup_dir/docker-compose-down.sh"
rm -f "$GIZCLAW_E2E_DOCKER_ENV"
cleanup_done=1

if [[ -n "$(docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.ID}}')" ]]; then
	echo "Gameplay deletion teardown left project containers" >&2
	exit 1
fi
if [[ -e "$state_dir" ]]; then
	echo "Gameplay deletion teardown left project runtime state" >&2
	exit 1
fi
