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

: "${GIZCLAW_LOCOMO_E2E_TEST_REGEX:?select one or more explicit TestLoCoMo... tests}"
: "${GIZCLAW_LOCOMO_E2E_DATASET:?set the converted Flowcraft eval JSONL dataset path}"
: "${GIZCLAW_LOCOMO_E2E_ANSWER_MODEL:?set the provider-independent answer model}"
: "${GIZCLAW_LOCOMO_E2E_OPENAI_API_KEY:?set the OpenAI-compatible credential used by answer/judge and Flowcraft profiles}"

if [[ "${GIZCLAW_LOCOMO_E2E_DATASET}" != "synthetic" && ! -f "${repo_root}/${GIZCLAW_LOCOMO_E2E_DATASET}" && ! -f "${GIZCLAW_LOCOMO_E2E_DATASET}" ]]; then
  echo "dataset not found: ${GIZCLAW_LOCOMO_E2E_DATASET}" >&2
  exit 1
fi

cd "${repo_root}"
go test -count=1 -v -tags gizclaw_locomo_e2e -run "${GIZCLAW_LOCOMO_E2E_TEST_REGEX}" ./tests/locomo-e2e
