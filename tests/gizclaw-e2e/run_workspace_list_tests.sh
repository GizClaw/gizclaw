#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-workspace-list.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/build-admission-runners.sh"
export GIZCLAW_WORKSPACE_LIST_C_RUNNER="$GIZCLAW_ADMISSION_C_RUNNER"
export GIZCLAW_WORKSPACE_LIST_FLUTTER_RUNNER="$GIZCLAW_ADMISSION_FLUTTER_RUNNER"
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
  -run '^TestWorkspaceList(GiztestGo|SDKGiztests)$' -count=1 -timeout=10m
