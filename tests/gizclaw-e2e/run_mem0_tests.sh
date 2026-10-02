#!/usr/bin/env bash
# Real extraction/vector recall and Peer RPC on the same PostgreSQL instance.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/mem0-giztest-XXXXXX")"
project="$(basename "$run_dir" | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_CREDENTIAL_FILE="${GIZCLAW_E2E_CREDENTIAL_FILE:-$script_dir/.env}"
if [[ ! -f "$GIZCLAW_E2E_CREDENTIAL_FILE" ]]; then
  : "${GIZCLAW_E2E_VOLC_ARK_API_KEY:?set the E2E Ark credential or provide the standard .env file}"
  GIZCLAW_E2E_CREDENTIAL_FILE="$run_dir/credentials.env"
  (umask 077; printf 'GIZCLAW_E2E_VOLC_ARK_API_KEY=%s\n' "$GIZCLAW_E2E_VOLC_ARK_API_KEY" > "$GIZCLAW_E2E_CREDENTIAL_FILE")
fi
export GIZCLAW_E2E_MEM0_API_KEY
GIZCLAW_E2E_MEM0_API_KEY="$(python3 -c 'import secrets; print(secrets.token_hex(24))')"
platform="$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}')"
export GIZCLAW_E2E_MEM0_BASE_IMAGE="${GIZCLAW_E2E_MEM0_BASE_IMAGE:-gizclaw-mem0:${platform//\//-}-base}"
cat > "$run_dir/ports.yaml" <<'PORTS'
services:
  postgres:
    ports: ["127.0.0.1::5432"]
  mem0:
    ports: ["127.0.0.1::8000"]
PORTS
compose=(docker compose --project-name "$project" --file "$script_dir/docker/compose.memory.yaml" --file "$run_dir/ports.yaml")
cleanup() {
  "${compose[@]}" logs --no-color > "$run_dir/containers.log" 2>&1 || true
  "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  rm -f "$run_dir/credentials.env"
  if [[ -n "$(docker ps -aq --filter "label=com.docker.compose.project=$project")" ||
        -n "$(docker volume ls -q --filter "label=com.docker.compose.project=$project")" ||
        -n "$(docker network ls -q --filter "label=com.docker.compose.project=$project")" ]]; then
    echo "Mem0 test project resources remain: $project" >&2
    return 1
  fi
  echo "Mem0 Giztest evidence: $run_dir"
}
trap cleanup EXIT
cd "$repo_root"
TARGET=base PLATFORM="$platform" BASE_IMAGE="$GIZCLAW_E2E_MEM0_BASE_IMAGE" "$repo_root/build/build-mem0.sh"
"${compose[@]}" build mem0
"${compose[@]}" up --detach --wait --wait-timeout 180 mem0
GIZCLAW_TEST_MEM0_ENDPOINT="http://$("${compose[@]}" port mem0 8000)"
GIZCLAW_TEST_POSTGRES_DSN="postgres://gizclaw_server:gizclaw_server@$("${compose[@]}" port postgres 5432)/gizclaw_server?sslmode=disable"
export GIZCLAW_TEST_MEM0_ENDPOINT GIZCLAW_TEST_POSTGRES_DSN
npm ci
npm run build:console
go test -v -count=1 -timeout=10m -run '^TestSelfHostedMem0Giztest$' ./cmd/internal/server | tee "$run_dir/giztest.log"
# Peer deletion must have emptied the exact vector collection used above.
"${compose[@]}" exec -T postgres psql -U gizclaw_bootstrap -d gizclaw_mem0 -v ON_ERROR_STOP=1 -c \
  "DO \$\$ BEGIN IF (SELECT count(*) FROM gizclaw_memory_doubao_v1) <> 0 THEN RAISE EXCEPTION 'memory residual after Peer cleanup'; END IF; END \$\$;"
export GIZCLAW_MEMORY_PROVIDER=mem0-self-hosted
export GIZCLAW_MEM0_SELF_HOSTED_URL="$GIZCLAW_TEST_MEM0_ENDPOINT"
export GIZCLAW_MEM0_SELF_HOSTED_API_KEY="$GIZCLAW_E2E_MEM0_API_KEY"
go test -v -tags=store_e2e -count=1 -timeout=5m -run '^TestSelfHostedMemoryLayoutScopeRouting$' ./tests/store-e2e | tee "$run_dir/scope.log"
# One PostgreSQL instance, two databases and distinct non-superuser roles.
"${compose[@]}" exec -T postgres psql -U gizclaw_bootstrap -d postgres -v ON_ERROR_STOP=1 -c \
  "DO \$\$ BEGIN IF EXISTS (SELECT FROM pg_roles WHERE rolname IN ('gizclaw_server','gizclaw_mem0') AND rolsuper) OR has_database_privilege('gizclaw_server','gizclaw_mem0','CONNECT') OR has_database_privilege('gizclaw_mem0','gizclaw_server','CONNECT') THEN RAISE EXCEPTION 'application role isolation failed'; END IF; END \$\$;"
