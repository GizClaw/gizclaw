#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-firmware-metadata.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
# Build the same native SDK runners used by the provider-free admission lane.
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/build-admission-runners.sh"
export GIZCLAW_FIRMWARE_METADATA_C_RUNNER="$GIZCLAW_ADMISSION_C_RUNNER"
export GIZCLAW_FIRMWARE_METADATA_FLUTTER_RUNNER="$GIZCLAW_ADMISSION_FLUTTER_RUNNER"
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
  -run '^TestFirmwareMetadata(Giztest|SDKGiztests)$' -count=1 -timeout=10m
