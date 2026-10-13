#!/usr/bin/env bash
# Isolated real Server/Redis/two-PostgreSQL recovery acceptance; no provider credentials.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
project="gizclaw-pending-recovery-${RANDOM}-$$"
export RECOVERY_ROOT="$script_dir/.testbench/$project"
export RECOVERY_POSTGRES_IMAGE="${RECOVERY_POSTGRES_IMAGE:-gizclaw-pending-recovery-postgres}"
RECOVERY_UID="$(id -u)"
RECOVERY_GID="$(id -g)"
export RECOVERY_UID RECOVERY_GID
architecture="$(docker info --format '{{.Architecture}}')"
case "$architecture" in aarch64|arm64) platform=linux-arm64;; x86_64|amd64) platform=linux-amd64;; *) echo 'unsupported Docker architecture' >&2;exit 2;; esac
export RECOVERY_BASE_IMAGE="${RECOVERY_BASE_IMAGE:-gizclaw-go:$platform-base}"
RECOVERY_PG_PASSWORD="$(openssl rand -hex 24)"
export RECOVERY_PG_PASSWORD
compose() { docker compose -p "$project" -f "$script_dir/docker/compose.pending-recovery.yaml" "$@"; }
cleanup() {
  status=$?
  if ((status != 0)); then compose logs --no-color --tail 100 2>&1 | python3 -B "$script_dir/setup/redact_diagnostics.py" >&2 || true; fi
  if ! compose down --volumes --remove-orphans >/dev/null 2>&1; then status=1; fi
  rm -rf "${RECOVERY_ROOT:?}/bin" "${RECOVERY_ROOT:?}/server-a" "${RECOVERY_ROOT:?}/server-b"
  exit "$status"
}
trap cleanup EXIT
mkdir -p "${RECOVERY_ROOT:?}/bin" "${RECOVERY_ROOT:?}/server-a" "${RECOVERY_ROOT:?}/server-b"
if ! docker image inspect "$RECOVERY_BASE_IMAGE" >/dev/null 2>&1; then
  docker build -f "$repo_root/build/gizclaw/Dockerfile.base" -t "$RECOVERY_BASE_IMAGE" "$repo_root/build"
fi
docker build -f "$repo_root/build/postgres/Dockerfile" -t "$RECOVERY_POSTGRES_IMAGE" "$repo_root"
(cd "$repo_root" && npm ci && npm run build:console)
docker run --rm --entrypoint /bin/bash \
  -v "$repo_root:/src" -v "$RECOVERY_ROOT/bin:/output" \
  -v "$(go env GOMODCACHE):/root/go/pkg/mod" -v "$(go env GOCACHE):/root/.cache/go-build" \
  "$RECOVERY_BASE_IMAGE" -lc 'cd /src && go build -o /output/gizclaw ./cmd/gizclaw && go test -c -tags gizclaw_e2e -o /output/recovery.test ./tests/gizclaw-e2e/go/recovery'
python3 - "$repo_root" <<'PY'
from pathlib import Path
import os,re,sys
root=Path(os.environ['RECOVERY_ROOT'])
template=(Path(sys.argv[1])/'tests/gizclaw-e2e/testdata/server-workspace/config.yaml.template').read_text()
for name,private,admin in [('a','287thuEga1hbBYjM7sxrY3eMoPAqpB3nJ81S8CEuNHKu','CdSRDESnon72PG3Cobd3G518f6NCNkA62nQ5kBrNCyz2'),('b','Get8yS32Es6muYoEbUgCQt15wF1Hcyc5sLRn7egBKCD1','9sie57gGyik7yWDwWQMCYPGyCLDr9FX7MvJkkZjtriJD')]:
 text=template.replace('peer-admission: ${GIZCLAW_E2E_PEER_ADMISSION}\n','')
 text=text.replace('private-key: 287thuEga1hbBYjM7sxrY3eMoPAqpB3nJ81S8CEuNHKu','private-key: '+private)
 text=text.replace('${GIZCLAW_E2E_SERVER_ENDPOINT}',f'server-{name}:9820')
 text=re.sub(r'ice-servers:\n.*?(?=admin-public-key:)', '',text,flags=re.S)
 text=text.replace('admin-public-key: "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"',f'admin-public-key: "{admin}"')
 text=text.replace('storage:\n','storage:\n  shared-redis:\n    kind: redis.readwrite\n    read_url: redis://replica:6379/0\n    write_url: redis://redis:6379/0\n',1)
 text=text.replace('kind: sqlite\n    dir: data/business.sqlite',f"kind: postgresql\n    dsn: postgres://recovery:{os.environ['RECOVERY_PG_PASSWORD']}@pg-{name}:5432/recovery?sslmode=disable")
 for store in ['peers','api-keys','friends','friend-groups']:
  text=text.replace(f'  {store}:\n    kind: keyvalue\n    storage: memory',f'  {store}:\n    kind: keyvalue\n    storage: shared-redis')
 text+='\npending_deletion:\n  scan_interval: 250ms\n  page_size: 20\n  workers: 2\n  lease_duration: 12s\n  attempt_timeout: 8s\n  retry_initial: 250ms\n  retry_max: 1s\n  max_attempts: 3\n'
 target=root/f'server-{name}'/'config.yaml';target.write_text(text);target.chmod(0o600)
PY
compose up -d --wait redis replica pg-a pg-b
for region in a b; do
  compose exec -T "pg-$region" psql -U recovery -d recovery -v ON_ERROR_STOP=1 -c 'CREATE SCHEMA partman' -c 'CREATE EXTENSION pg_partman WITH SCHEMA partman'
done
compose up -d --wait server-a server-b
compose run --rm tests -test.run '^(TestRegionalRecovery|TestOnlineRecoveryCLI)$' | tee "$RECOVERY_ROOT/regional.log"
compose run --rm -e RECOVERY_STAGE=prepare-interruption tests -test.run '^TestPrepareInterruptedRecovery$' >"$RECOVERY_ROOT/interruption.log" 2>&1 &
preparer=$!
ready=0
for _ in {1..300}; do
  if [[ -s "$RECOVERY_ROOT/interrupted.json" ]]; then ready=1;break;fi
  if ! kill -0 "$preparer" 2>/dev/null;then cat "$RECOVERY_ROOT/interruption.log";wait "$preparer";exit 1;fi
  sleep 0.1
done
if ((ready == 0));then echo 'worker did not reach interruption point' >&2;exit 1;fi
compose kill -s SIGKILL server-a
touch "$RECOVERY_ROOT/server-killed"
wait "$preparer"
cat "$RECOVERY_ROOT/interruption.log"
# The killed process wrote PID 1; a new container also uses PID 1. Remove
# only this known-dead fixture's process lock after verifying it is stopped.
server_container="$(compose ps -aq server-a)"
if [[ "$(docker inspect -f '{{.State.Running}}' "$server_container")" != false ]]; then
  echo 'home Server is still running after kill' >&2; exit 1
fi
rm -f "${RECOVERY_ROOT:?}/server-a/serve.pid"
compose up -d --wait server-a
compose run --rm -e RECOVERY_STAGE=resume-interruption tests -test.run '^TestResumeInterruptedRecovery$' | tee "$RECOVERY_ROOT/resume.log"
compose run --rm tests -test.run '^TestRecoveryReadsCentralWriterDespiteStaleReplica$' | tee "$RECOVERY_ROOT/primary.log"
echo "Recovery evidence: $RECOVERY_ROOT"
