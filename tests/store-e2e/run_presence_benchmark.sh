#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
if [ "$#" -lt 2 ] || [ "$#" -gt 4 ]; then
  echo 'usage: run_presence_benchmark.sh <new-evidence-dir> <baseline-ref> [repetitions=3] [duration=15s]' >&2
  exit 2
fi
evidence="$1"
baseline="$(git rev-parse --verify "$2^{commit}")"
repetitions="${3:-3}"
duration="${4:-15s}"
case "$repetitions" in [1-9]) ;; *) echo 'repetitions must be 1-9' >&2; exit 2 ;; esac
if [ -e "$evidence" ]; then echo 'evidence directory must be new' >&2; exit 2; fi
mkdir -p "$evidence/bin" "$evidence/baseline" "$evidence/results"
evidence="$(cd "$evidence" && pwd)"
export PRESENCE_BENCHMARK_DIR="$evidence"
compose_file="$repo_root/tests/store-e2e/docker-compose.presence-benchmark.yaml"
project="gizclaw-presence-benchmark-$(date +%s)-$$"
printf '%s\n' "$project" >"$evidence/project.txt"
compose() { docker compose -p "$project" -f "$compose_file" "$@"; }
pair_dir="$evidence"
cleanup() {
  compose logs --no-color >"$pair_dir/cluster.log" 2>&1 || true
  compose down --timeout 3 --volumes --remove-orphans >"$pair_dir/cleanup.log" 2>&1
}
trap cleanup EXIT

# Use the identical load driver and dependencies for both variants. Overlay
# only the production files this PR changes; reject incomplete comparisons.
python3 - "$repo_root" "$evidence" "$baseline" <<'PY'
import json
import pathlib
import subprocess
import sys

root, output, baseline = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]), sys.argv[3]
files = {
    'pkgs/gizclaw/services/runtime/peerpresence/presence.go',
    'pkgs/gizclaw/services/runtime/peerpresence/transitions.go',
    'pkgs/store/kv/sets_tikv.go',
}
changed = subprocess.check_output(['git', 'diff', '--name-only', baseline, 'HEAD', '--', 'pkgs/gizclaw/services/runtime/peerpresence', 'pkgs/store/kv'], text=True).splitlines()
unexpected = [p for p in changed if p.endswith('.go') and not p.endswith('_test.go') and p not in files]
if unexpected:
    raise SystemExit('comparison needs additional baseline production files: ' + ', '.join(unexpected))
replacements = {}
for i, relative in enumerate(sorted(files)):
    saved = output / 'baseline' / f'{i}.go'
    source = subprocess.run(['git', 'show', f'{baseline}:{relative}'], capture_output=True)
    if source.returncode:
        if relative.endswith('/transitions.go'):
            saved.write_text('package peerpresence\n')
        else:
            raise SystemExit(f'missing baseline source: {relative}')
    else:
        saved.write_bytes(source.stdout)
    replacements[str(root / relative)] = str(saved)
(output / 'baseline' / 'overlay.json').write_text(json.dumps({'Replace': replacements}, indent=2) + '\n')
manifest = {'baseline': baseline, 'fixed': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(), 'comparison': 'identical driver and dependencies; baseline production files overlaid', 'overlay_files': sorted(files)}
(output / 'source-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
PY
git diff HEAD >"$evidence/source.patch"
git status --short >"$evidence/source-status.txt"
go version >"$evidence/go-version.txt"
cp tests/store-e2e/presence-benchmark/main.go "$compose_file" tests/store-e2e/tikv-test.toml "$evidence/"
architecture="$(docker info --format '{{.Architecture}}')"
case "$architecture" in
  arm64|aarch64) architecture=arm64 ;;
  amd64|x86_64) architecture=amd64 ;;
  *) echo 'unsupported Docker architecture' >&2; exit 2 ;;
esac
GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 go build -o "$evidence/bin/fixed" ./tests/store-e2e/presence-benchmark
GOOS=linux GOARCH="$architecture" CGO_ENABLED=0 go build -overlay="$evidence/baseline/overlay.json" -o "$evidence/bin/baseline" ./tests/store-e2e/presence-benchmark
shasum -a 256 "$evidence/bin/fixed" "$evidence/bin/baseline" >"$evidence/binary-sha256.txt"

# Each pair gets a fresh three-voter cluster; AB/BA order alternates. No host
# ports or online endpoints are used. A single pair runs at a time.
cases="${PRESENCE_BENCHMARK_CASES:-1:0ms:1 1:20ms:1 120:0ms:1 120:20ms:1 250:0ms:1 250:20ms:1 500:0ms:1 500:20ms:1 500:20ms:3}"
for specification in $cases; do
  IFS=: read -r peers delay servers <<<"$specification"
  for ((repeat=1; repeat<=repetitions; repeat++)); do
    pair="peers-$peers-delay-$delay-servers-$servers-repeat-$repeat"
    pair_dir="$evidence/results/$pair"
    mkdir -p "$pair_dir"
    printf 'starting %s\n' "$pair"
    compose up -d pd voter-a voter-b voter-c >"$pair_dir/startup.log" 2>&1
    compose ps --format json >"$pair_dir/topology.json"
    if (( repeat % 2 == 1 )); then order='baseline fixed'; else order='fixed baseline'; fi
    for variant in $order; do
      printf 'running %s %s\n' "$pair" "$variant"
      compose run --rm --no-deps --entrypoint "/bench/bin/$variant" benchmark \
        -pd=pd:2379 -peers="$peers" -servers="$servers" -delay="$delay" -duration="$duration" \
        -output="/bench/results/$pair/$variant.json" -label="$variant/$pair" \
        >"$pair_dir/$variant.log" 2>&1
      python3 - "$pair_dir/$variant.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
p95 = r['completed_latency_p95_ms']
latency = 'no completions' if p95 is None else f'{p95:.2f}ms'
print(f"{r['label']}: submitted={r['submitted']} completed={r['completed_including_drain']} within2s={r['completed_within_2s']} incomplete={r['uncompleted_after_drain']} throughput={r['window_completions_per_second']:.2f}/s completed_p95={latency} prewrites={r['prewrite_rpc_calls']} conflicts={r['prewrite_write_conflict_errors']}")
PY
    done
    cleanup
  done
done
trap - EXIT
printf 'benchmark complete: %s\n' "$evidence"
