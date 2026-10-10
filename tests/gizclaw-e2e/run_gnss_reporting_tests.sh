#!/usr/bin/env bash
# GNSS invoke Giztests through real Server/Edge and all four native SDK runners.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-gnss-reporting.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
# Reuse the native SDK runner builds used by the admission lanes.
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/build-admission-runners.sh"
export GIZCLAW_GNSS_C_RUNNER="$GIZCLAW_ADMISSION_C_RUNNER"
export GIZCLAW_GNSS_FLUTTER_RUNNER="$GIZCLAW_ADMISSION_FLUTTER_RUNNER"
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
  -run '^TestGNSSReporting(GiztestGo|SDKGiztests)$' -count=1 -timeout=10m
