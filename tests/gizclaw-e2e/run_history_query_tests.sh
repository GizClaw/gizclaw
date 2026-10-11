#!/usr/bin/env bash
set -euo pipefail

# Compare only the released LogStore implementation with the working tree.
# Server, Eino, Giztest, seeded rows and concurrency are identical in both runs.
root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)
cd "$root"
mkdir -p "$root/.tmp"
: "${GIZCLAW_TEST_POSTGRES_DSN:?set a dedicated local PostgreSQL 17 URL}"
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
if [[ -e "$evidence/source.json" || -e "$evidence/before" || -e "$evidence/after" ]]; then
  printf 'Choose a fresh evidence directory: %s\n' "$evidence" >&2
  exit 1
fi
base_ref=${GIZCLAW_TEST_HISTORY_QUERY_BASE_REF:-v0.31.1}
base_commit=$(git rev-parse "${base_ref}^{commit}")
schema="gzc_history_query_$(date +%s)_${RANDOM}"
mkdir -p "$evidence/base-source"
git show "$base_commit:pkgs/store/logstore/sql.go" > "$evidence/base-source/sql.go"
git show "$base_commit:pkgs/store/logstore/sql_schema.go" > "$evidence/base-source/sql_schema.go"
python3 - "$root" "$evidence" "$base_commit" "$schema" <<'PY'
import hashlib, json, pathlib, subprocess, sys
root, evidence = map(pathlib.Path, sys.argv[1:3])
files = ('sql.go', 'sql_schema.go')
overlay = {str(root/'pkgs/store/logstore'/name): str(evidence/'base-source'/name) for name in files}
(evidence/'overlay.json').write_text(json.dumps({'Replace': overlay}))
source = {}
for kind, parent in [('baseline', evidence/'base-source'), ('working', root/'pkgs/store/logstore')]:
    source[kind] = {name: hashlib.sha256((parent/name).read_bytes()).hexdigest() for name in files}
(evidence/'source.json').write_text(json.dumps({
    'baseline_commit':sys.argv[3], 'working_head':subprocess.check_output(['git','rev-parse','HEAD'], text=True).strip(),
    'schema':sys.argv[4], 'source_sha256':source,
    'baseline_scope':'only LogStore sql.go and sql_schema.go; all other code uses the working tree',
}, indent=2))
PY

export GIZCLAW_TEST_HISTORY_QUERY_LOAD=1
export GIZCLAW_TEST_HISTORY_QUERY_SCHEMA="$schema"
export GIZCLAW_TEST_HISTORY_GIZTEST_EVIDENCE="$evidence/before"
set +e
go test -overlay "$evidence/overlay.json" -count=1 -timeout=12m -v \
  -run '^TestPostgreSQLHistoryQueryGiztest$' ./cmd/internal/server > "$evidence/before-test.log" 2>&1
baseline_status=$?
set -e
test -s "$evidence/before/giztest.json"

export GIZCLAW_TEST_HISTORY_GIZTEST_EVIDENCE="$evidence/after"
set +e
go test -count=1 -timeout=12m -v -run '^TestPostgreSQLHistoryQueryGiztest$' \
  ./cmd/internal/server > "$evidence/after-test.log" 2>&1
fixed_status=$?
set -e
python3 - "$evidence" "$baseline_status" "$fixed_status" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
summary = {'baseline_exit':int(sys.argv[2]), 'fixed_exit':int(sys.argv[3])}
def read(folder, name):
    return json.loads((folder/name).read_text())
def walk(node):
    yield node
    for child in node.get('Plans', []):
        yield from walk(child)
for phase in ('before', 'after'):
    folder = root/phase
    report = read(folder, 'giztest.json')
    load = read(folder, 'query-load.json')
    plans = {}
    for name in ('existing', 'missing'):
        plan = read(folder, name+'.plan.json')[0]
        nodes = list(walk(plan['Plan']))
        plans[name] = {
            'execution_ms':plan['Execution Time'], 'planning_ms':plan['Planning Time'],
            'heap_rows_examined':sum((n.get('Actual Rows',0)+n.get('Rows Removed by Filter',0)+n.get('Rows Removed by Index Recheck',0))*n.get('Actual Loops',0) for n in nodes if n['Node Type'] in ('Index Scan','Seq Scan','Bitmap Heap Scan')),
            'json_function_loops':sum(n.get('Actual Loops',0) for n in nodes if n['Node Type']=='Function Scan'),
            'shared_hit_blocks':plan['Plan'].get('Shared Hit Blocks',0),
            'shared_read_blocks':plan['Plan'].get('Shared Read Blocks',0),
            'sorts':[{'method':n.get('Sort Method'), 'space_kb':n.get('Sort Space Used')} for n in nodes if n['Node Type']=='Sort'],
        }
    summary[phase] = {
        'fixture':read(folder, 'fixture.json'), 'plans':plans,
        'query_latency':load['latency'], 'queries_per_second':load['queries_per_second'],
        'pool_wait_count':load['pool_wait_count'], 'pool_wait_ms':load['pool_wait_ms'],
        'write_latency':read(folder, 'write-load.json')['latency'],
        'giztest_write_latency':read(folder, 'giztest-writes.json')['latency'],
        'pool_probe_latency':read(folder, 'pool-probes.json')['latency'],
        'giztest_status':report['status'], 'giztest_tasks':len(report['tasks']),
        'giztest_first_text_step_ms':[next(s['duration_ms'] for s in task['steps'] if s['id']=='first_text') for task in report['tasks']],
    }
if summary['before']['fixture'] != summary['after']['fixture']:
    raise SystemExit('fixture changed between baseline and fixed runs')
(root/'comparison.json').write_text(json.dumps(summary, indent=2))
print(json.dumps(summary, indent=2))
if summary['fixed_exit'] or summary['after']['giztest_status'] != 'passed':
    raise SystemExit('fixed Giztest failed; inspect evidence')
for metric in ('query_latency', 'write_latency', 'giztest_write_latency', 'pool_probe_latency'):
    if summary['after'][metric]['errors']:
        raise SystemExit('fixed '+metric+' contains errors; inspect evidence')
if not summary['after']['giztest_write_latency']['count']:
    raise SystemExit('fixed Giztest had no concurrent writes')
PY
printf '\nEvidence: %s\nRetained synthetic fixture schema: %s\n' "$evidence" "$schema"
