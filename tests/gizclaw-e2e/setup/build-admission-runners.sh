#!/usr/bin/env bash
# Sourced by admission lanes; repo_root and task_dir belong to the caller.
: "${repo_root:?caller must set repo_root}"
: "${task_dir:?caller must set task_dir}"
for tool in go node npm protoc flutter dart; do
  command -v "$tool" >/dev/null || { echo "required tool missing: $tool" >&2; exit 1; }
done
case "$(uname -s)" in
  Darwin) target=macos ;;
  Linux) target=linux ;;
  *) echo "admission SDK lane requires macOS or Linux" >&2; exit 1 ;;
esac
# Build every runner for both admission hosts; missing tools fail closed.
npm ci
npm run build:console
npm --prefix sdk/js/gizclaw run build
npm --prefix sdk/js/gizclaw-control run build
npm --prefix tests/gizclaw-e2e/js run prepare:giztest
git submodule update --init --depth=1 third_party/nanopb/upstream
go build -o "$task_dir/giztest-c" ./tests/gizclaw-e2e/cgo/giztest
export GIZCLAW_ADMISSION_C_RUNNER="$task_dir/giztest-c"
package_dir="$repo_root/tests/gizclaw-e2e/flutter/giztest"
(cd "$package_dir" && flutter pub get)
if [[ "$target" == linux && "${GIZCLAW_NATIVE_CHANNEL_FIX:-0}" == 1 ]]; then
  python3 - "$package_dir" "$task_dir" "$repo_root/tests/gizclaw-e2e/testdata/native-flutter-close.patch" <<'PY'
import json
from pathlib import Path
import shutil
import subprocess
import sys
from urllib.parse import unquote, urljoin, urlparse

package, task, patch = map(Path, sys.argv[1:])
config = package / ".dart_tool/package_config.json"
entry = next(p for p in json.loads(config.read_text())["packages"] if p["name"] == "flutter_webrtc")
uri = urlparse(urljoin(config.resolve().as_uri(), entry["rootUri"]))
assert uri.scheme == "file"
source = Path(unquote(uri.path))
destination = task / "flutter_webrtc"
shutil.copytree(source, destination, symlinks=True)
subprocess.run(["git", "-C", str(destination), "apply", "--check", str(patch)], check=True)
subprocess.run(["git", "-C", str(destination), "apply", str(patch)], check=True)
(package / "pubspec_overrides.yaml").write_text(json.dumps({
    "dependency_overrides": {"flutter_webrtc": {"path": str(destination)}}
}))
PY
  (cd "$package_dir" && flutter pub get)
fi
(cd "$package_dir" && flutter build "$target" --debug)
if [[ "$target" == macos ]]; then
  export GIZCLAW_ADMISSION_FLUTTER_RUNNER="$package_dir/build/macos/Build/Products/Debug/giztest.app/Contents/MacOS/giztest"
else
  export GIZCLAW_ADMISSION_FLUTTER_RUNNER="$package_dir/build/linux/x64/debug/bundle/giztest"
fi
