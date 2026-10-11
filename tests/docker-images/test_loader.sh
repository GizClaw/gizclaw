#!/usr/bin/env bash
# Exercise download/import/cache wiring with recorded curl and Docker fixtures.
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mkdir -p "$repo_root/.tmp"
work="$(mktemp -d "$repo_root/.tmp/image-loader-test.XXXXXX")"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"
export MOCK_DOCKER_STATE="$work/docker-state" MOCK_COMMANDS="$work/commands"
export GIZCLAW_CONTAINER_IMAGE_CACHE_DIR="$work/cache"
unset GIZCLAW_CONTAINER_CACHE_ONLY
cat > "$work/bin/docker" <<'DOCKER'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >> "$MOCK_COMMANDS"
case "$1 ${2:-}" in
  'image inspect') [[ -f "$MOCK_DOCKER_STATE" ]] ;;
  'info --format') echo aarch64 ;;
  'load --input')
    [[ "${MOCK_LOAD_FAIL:-0}" != 1 ]] || exit 9
    [[ -s "$3" ]]
    touch "$MOCK_DOCKER_STATE"
    echo 'Loaded fixture image'
    ;;
  *) echo 'unexpected Docker operation' >&2; exit 99 ;;
esac
DOCKER
cat > "$work/bin/curl" <<'CURL'
#!/usr/bin/env bash
set -euo pipefail
output="" url="" fail_flag=0
while (($#)); do
  case "$1" in
    --output) output="$2"; shift 2 ;;
    --fail) fail_flag=1; shift ;;
    https://*) url="$1"; shift ;;
    *) shift ;;
  esac
done
[[ "$fail_flag" == 1 && -n "$output" && -n "$url" ]]
printf 'curl %s\n' "$url" >> "$MOCK_COMMANDS"
[[ "${MOCK_CURL_FAIL:-0}" != 1 ]] || exit 22
printf 'Docker archive fixture\n' > "$output"
CURL
chmod +x "$work/bin/docker" "$work/bin/curl"
export PATH="$work/bin:$PATH"
loader="$repo_root/tests/docker-images/load.sh"
# shellcheck source=common.sh
# shellcheck disable=SC1091
source "$repo_root/tests/docker-images/common.sh"
key=dc38bbda7b864a80b4d695b29736bc808c077014fcea0566b21a674f50faa5d5
expected="gizclaw-tos/cache:$key-amd64"
archive="$work/cache/$key/amd64/image.tar.gz"
count_command() { grep -c "^$1 " "$MOCK_COMMANDS" || true; }
reset_case() { rm -f "$MOCK_DOCKER_STATE"; rm -rf "$work/cache"; : > "$MOCK_COMMANDS"; }
reset_case
[[ "$(bash "$loader" ubuntu amd64 2> "$work/stderr")" == "$expected" ]]
[[ "$(count_command curl)" == 1 && -s "$archive" ]]
grep -Fxq "curl https://static-volc.gizclaw.com/releases/GizClaw/deploy/container-images/v1/$key/amd64/image.tar.gz" "$MOCK_COMMANDS"
echo 'PASS direct CDN archive download and Docker import'
[[ "$(bash "$loader" ubuntu amd64 2> "$work/stderr")" == "$expected" ]]
[[ "$(count_command curl)" == 1 && "$(grep -c '^docker load ' "$MOCK_COMMANDS")" == 1 ]]
echo 'PASS already loaded image reuse'
rm -f "$MOCK_DOCKER_STATE"
[[ "$(bash "$loader" ubuntu amd64 2> "$work/stderr")" == "$expected" ]]
[[ "$(count_command curl)" == 1 && "$(grep -c '^docker load ' "$MOCK_COMMANDS")" == 2 ]]
echo 'PASS cached archive import without download'
reset_case
if MOCK_CURL_FAIL=1 bash "$loader" ubuntu amd64 > "$work/stdout" 2> "$work/stderr"; then exit 1; fi
[[ ! -e "$archive" && ! -e "$MOCK_DOCKER_STATE" ]]
[[ -z "$(find "$work/cache" -name '.download-*' -print)" ]]
echo 'PASS download failure and partial-file cleanup'
reset_case
if MOCK_LOAD_FAIL=1 bash "$loader" ubuntu amd64 > "$work/stdout" 2> "$work/stderr"; then exit 1; fi
[[ -f "$archive" && ! -e "$MOCK_DOCKER_STATE" ]]
echo 'PASS Docker import failure propagation'
reset_case
if GIZCLAW_CONTAINER_CACHE_ONLY=1 bash "$loader" ubuntu amd64 > "$work/stdout" 2> "$work/stderr"; then exit 1; fi
[[ "$(count_command curl)" == 0 ]]
echo 'PASS cache-only mode fails without a cached image/archive'
reset_case
if bash "$loader" absent amd64 > "$work/stdout" 2> "$work/stderr"; then exit 1; fi
if bash "$loader" ubuntu unsupported > "$work/stdout" 2> "$work/stderr"; then exit 1; fi
[[ "$(count_command curl)" == 0 ]]
echo 'PASS unknown image and unsupported architecture'
[[ "$(bash "$loader" ubuntu 2> "$work/stderr")" == "gizclaw-tos/cache:$key-arm64" ]]
gizclaw_image_env arm64
[[ "$GIZCLAW_E2E_UBUNTU_IMAGE" == "gizclaw-tos/cache:$key-arm64" ]]
[[ "$(gizclaw_image_sources | wc -l | tr -d ' ')" == 8 ]]
echo 'PASS native architecture selection and Compose environment'
