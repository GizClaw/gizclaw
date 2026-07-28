#!/usr/bin/env bash
set -euo pipefail

selection="$1"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
dataset="${GIZCLAW_LOCOMO_E2E_DATASET:-tests/locomo-e2e/testdata/locomo10_smoke.jsonl}"
test_timeout="${GIZCLAW_LOCOMO_E2E_TEST_TIMEOUT:-30m}"

if [[ ! -f "$dataset" && ! -f "$repo_root/$dataset" ]]; then
  echo "dataset not found: $dataset" >&2
  exit 2
fi

cd "$repo_root"
go test -count=1 -timeout "$test_timeout" -v -tags gizclaw_locomo_e2e \
  -run "$selection" ./tests/locomo-e2e
