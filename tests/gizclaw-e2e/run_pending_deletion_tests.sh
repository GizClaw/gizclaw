#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
cleanup_done=0
project_suffix="$(printf '%s-%s' "${USER:-user}" "$$" | tr -cd '[:alnum:]-' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-pending-deletion-$project_suffix"
export GIZCLAW_E2E_DOCKER_ENV="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT.env"
export GIZCLAW_E2E_SYNC_VOLC_TENANT_ID=""
export GIZCLAW_E2E_PRESERVE_DATA_ON_RESTART=1
export GIZCLAW_E2E_PERSISTENT_KV=1
export GIZCLAW_E2E_RETAINED_PET_RESTART_STATE="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT/retained-pet-restart.json"

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

# shellcheck disable=SC2329 # Invoked by the EXIT trap.
cleanup() {
	local status="$?"
	if [[ "$cleanup_done" == "0" && -f "$GIZCLAW_E2E_DOCKER_ENV" ]]; then
		if [[ "$status" != "0" ]]; then
			local server_container
			server_container="$(docker ps -aq \
				--filter "label=com.docker.compose.project=$GIZCLAW_E2E_DOCKER_PROJECT" \
				--filter "label=com.docker.compose.service=server" | head -n 1)"
			if [[ -n "$server_container" ]]; then
				docker logs "$server_container" >&2 || true
			fi
		fi
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
export GIZCLAW_E2E_ADMIN_SANDBOX="$script_dir/testdata/docker/$GIZCLAW_E2E_DOCKER_PROJECT/pending-deletion-admin"

(cd "$repo_root" && go test -count=1 -v -tags gizclaw_e2e -timeout 12m \
	-run '^(TestPetDeletionDuringContinuousRPCUse|TestWorkspaceDeletionQuiescesRunningRuntime|TestFriendGroupDeletionQuiescesEveryMemberRuntime|TestPeerSelfDeletionStopsActiveConnectionAndRuntime|TestAdminPeerDeletionStopsActiveSession|TestPeerDeletionRetainsAlreadyDeletedPetWorkspaceForRestart)$' \
	./tests/gizclaw-e2e/go/delete)

compose_file="$script_dir/docker/docker-compose.yaml"
docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" restart server >/dev/null
wait_for_server_info() {
	local endpoint="$1"
	local label="$2"
	for _ in {1..180}; do
		if curl -fsS --max-time 1 "http://$endpoint/server-info" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	echo "pending deletion $label did not recover after restart" >&2
	return 1
}
wait_for_server_info "$GIZCLAW_E2E_SERVER_ENDPOINT" server
docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" -f "$compose_file" restart edge edge2 >/dev/null
wait_for_server_info "$GIZCLAW_E2E_EDGE_ENDPOINT" edge
wait_for_server_info "$GIZCLAW_E2E_EDGE2_ENDPOINT" edge2

(cd "$repo_root" && GIZCLAW_E2E_VERIFY_PEER_DELETION_RESTART=1 \
	go test -count=1 -v -tags gizclaw_e2e -timeout 2m \
	-run '^(TestPeerDeletionSurvivesServerRestart|TestDeletedPetWorkspaceDoesNotBlockServerRestart)$' \
	./tests/gizclaw-e2e/go/delete)

project="$GIZCLAW_E2E_DOCKER_PROJECT"
state_dir="$script_dir/testdata/docker/$project"
bash "$setup_dir/docker-compose-down.sh"
rm -f "$GIZCLAW_E2E_DOCKER_ENV"
cleanup_done=1

if [[ -n "$(docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.ID}}')" ]]; then
	echo "Pending deletion teardown left project containers" >&2
	exit 1
fi
if [[ -e "$state_dir" ]]; then
	echo "Pending deletion teardown left project runtime state" >&2
	exit 1
fi
