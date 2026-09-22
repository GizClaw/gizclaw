#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-admission.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT
# shellcheck source=setup/build-admission-runners.sh
# shellcheck disable=SC1091
source "$repo_root/tests/gizclaw-e2e/setup/build-admission-runners.sh"
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
  -run '^TestAdmission(GiztestGo|SDKGiztests)$' -count=1 -timeout=10m
go test -tags=giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^TestWebRTCStructuredAdmissionCredential$' -count=1
