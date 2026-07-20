#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../.." && pwd)"
env_file="${script_dir}/.env"

if [[ ! -f "${env_file}" ]]; then
  echo "missing ${env_file}; copy .env.example and fill the selected live profile" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1090
source "${env_file}"
set +a

: "${GIZCLAW_LOCOMO_E2E_TEST_REGEX:?select one explicit TestLoCoMo... test}"
: "${GIZCLAW_LOCOMO_E2E_MODEL_API_KEY:?set the Volcengine Ark API key used by Doubao}"

case "${GIZCLAW_LOCOMO_E2E_TEST_REGEX}" in
  TestLoCoMoFlowcraftBM25SinglePass | \
    TestLoCoMoFlowcraftHybridSinglePass | \
    TestLoCoMoFlowcraftHybridTwoPass | \
    TestLoCoMoMem0PlatformDefault | \
    TestLoCoMoMem0PlatformCustomInstructions | \
    TestLoCoMoVolcAgentKitDefault) ;;
  *)
    echo "GIZCLAW_LOCOMO_E2E_TEST_REGEX must name one supported live test exactly" >&2
    exit 1
    ;;
esac

dataset="${GIZCLAW_LOCOMO_E2E_DATASET:-tests/locomo-e2e/testdata/locomo10_smoke.jsonl}"
test_timeout="${GIZCLAW_LOCOMO_E2E_TEST_TIMEOUT:-30m}"
if [[ ! -f "${dataset}" && ! -f "${repo_root}/${dataset}" ]]; then
  echo "dataset not found: ${dataset}" >&2
  exit 1
fi

cd "${repo_root}"
go test \
  -count=1 \
  -timeout "${test_timeout}" \
  -v \
  -tags gizclaw_locomo_e2e \
  -run "^${GIZCLAW_LOCOMO_E2E_TEST_REGEX}$" \
  ./tests/locomo-e2e
