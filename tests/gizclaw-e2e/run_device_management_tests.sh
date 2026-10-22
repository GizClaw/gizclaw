#!/usr/bin/env bash
# Provider-free management, MHS and KV Giztests on real Server/Edge transports.
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-management.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/build-admission-runners.sh"
export GIZCLAW_MANAGEMENT_C_RUNNER="$GIZCLAW_ADMISSION_C_RUNNER"
export GIZCLAW_MANAGEMENT_FLUTTER_RUNNER="$GIZCLAW_ADMISSION_FLUTTER_RUNNER"
# Recreate native channels across three complete rounds. Any failed round
# fails the lane; this does not retry or discard a failed scenario.
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
 -run '^TestDeviceManagement(GiztestsGo|SDKGiztests)$' -count=3 -timeout=20m -v
