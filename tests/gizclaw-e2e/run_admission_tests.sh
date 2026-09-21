#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$repo_root"
for tool in go node npm protoc flutter dart; do
  command -v "$tool" >/dev/null || { echo "required tool missing: $tool" >&2; exit 1; }
done
case "$(uname -s)" in
  Darwin) target=macos ;;
  Linux) target=linux ;;
  *) echo "admission SDK lane requires macOS or Linux" >&2; exit 1 ;;
esac
task_dir="$(mktemp -d "${TMPDIR:-/tmp}/gizclaw-admission.XXXXXX")"
trap 'rm -rf "$task_dir"' EXIT

# This lane uses a real local Server and WebRTC peers, with temporary SQLite
# state and an explicitly provisioned administrative Peer. It needs no AI keys.
npm ci
npm run build:console
npm --prefix sdk/js/gizclaw run build
npm --prefix sdk/js/gizclaw-control run build
npm --prefix tests/gizclaw-e2e/js run prepare:giztest
git submodule update --init --depth=1 third_party/nanopb/upstream
go build -o "$task_dir/giztest-c" ./tests/gizclaw-e2e/cgo/giztest
export GIZCLAW_ADMISSION_C_RUNNER="$task_dir/giztest-c"
package_dir="$repo_root/tests/gizclaw-e2e/flutter/giztest"
(cd "$package_dir" && flutter pub get && flutter build "$target" --debug)
if [[ "$target" == macos ]]; then
  export GIZCLAW_ADMISSION_FLUTTER_RUNNER="$package_dir/build/macos/Build/Products/Debug/giztest.app/Contents/MacOS/giztest"
else
  export GIZCLAW_ADMISSION_FLUTTER_RUNNER="$package_dir/build/linux/x64/debug/bundle/giztest"
fi
go test -tags=gizclaw_sdk_e2e ./cmd/internal/server \
  -run '^TestAdmission(GiztestGo|SDKGiztests)$' -count=1 -timeout=10m
go test -tags=giznet_e2e ./tests/giznet-e2e/webrtc \
  -run '^TestWebRTCStructuredAdmissionCredential$' -count=1
