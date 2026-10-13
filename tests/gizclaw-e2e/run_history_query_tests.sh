#!/usr/bin/env bash
set -euo pipefail
# Exercise the column-based History implementation with real pg_partman/WebRTC.
root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$root"
mkdir -p "$root/.tmp"
: "${GIZCLAW_TEST_POSTGRES_DSN:?set a dedicated local PostgreSQL 17 URL with pg_partman}"
python3 - <<'PY'
import os
from urllib.parse import urlparse
url = urlparse(os.environ['GIZCLAW_TEST_POSTGRES_DSN'])
if url.scheme not in ('postgres', 'postgresql') or url.hostname not in ('localhost', '127.0.0.1', '::1'):
    raise SystemExit('history query fixture requires a loopback PostgreSQL URL')
PY
evidence=${1:-$(mktemp -d "$root/.tmp/history-query-XXXXXXXX")}
mkdir -p "$evidence"
evidence=$(cd "$evidence" && pwd)
if [[ -e "$evidence/source.json" || -e "$evidence/current" ]]; then
  printf 'Choose a fresh evidence directory: %s\n' "$evidence" >&2
  exit 1
fi
schema="gzc_history_query_$(date +%s)_${RANDOM}"
python3 - "$root" "$evidence" "$schema" <<'PY'
import hashlib, json, pathlib, subprocess, sys
root, evidence = map(pathlib.Path, sys.argv[1:3])
files = ('pkgs/store/memory/history.go','pkgs/store/memory/history_sql.go','pkgs/store/memory/history_partman.go','pkgs/genx/transformers/eino/history.go')
(evidence/'source.json').write_text(json.dumps({'working_head':subprocess.check_output(['git','rev-parse','HEAD'], text=True).strip(),'schema':sys.argv[3],'source_sha256':{name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in files}},indent=2))
PY
export GIZCLAW_TEST_HISTORY_QUERY_LOAD=1
export GIZCLAW_TEST_HISTORY_QUERY_SCHEMA="$schema"
export GIZCLAW_TEST_HISTORY_GIZTEST_EVIDENCE="$evidence/current"
set +e
go test -count=1 -timeout=12m -v -run '^TestPostgreSQLHistoryQueryGiztest$' \
  ./cmd/internal/server > "$evidence/test.log" 2>&1
status=$?
set -e
python3 - "$evidence" "$status" <<'PY'
import json, pathlib, sys
root=pathlib.Path(sys.argv[1]);folder=root/'current'
def read(name): return json.loads((folder/name).read_text())
report=read('giztest.json');load=read('query-load.json')
summary={'exit':int(sys.argv[2]),'fixture':read('fixture.json'),'query_latency':load['latency'],'queries_per_second':load['queries_per_second'],'pool_wait_ms':load['pool_wait_ms'],'giztest_status':report['status'],'giztest_tasks':len(report['tasks'])}
for field,name in [('write_latency','write-load.json'),('giztest_write_latency','giztest-writes.json'),('pool_probe_latency','pool-probes.json')]: summary[field]=read(name)['latency']
summary['plans']={name:read(name+'.plan.json') for name in ('existing','missing')}
(root/'summary.json').write_text(json.dumps(summary,indent=2))
print(json.dumps({key:value for key,value in summary.items() if key!='plans'},indent=2))
if summary['exit'] or report['status']!='passed': raise SystemExit('History load failed; inspect evidence')
for metric in ('query_latency','write_latency','giztest_write_latency','pool_probe_latency'):
    if summary[metric]['errors']: raise SystemExit(metric+' contains errors; inspect evidence')
if not summary['giztest_write_latency']['count']: raise SystemExit('Giztest had no concurrent writes')
PY
printf '\nEvidence: %s\nRetained synthetic fixture schema: %s\n' "$evidence" "$schema"
