#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
report="${1:?usage: run_docker_giztest.sh <report> <document>...}"
shift
if (($# == 0)); then
  echo "at least one Giztest document is required" >&2
  exit 2
fi
project="${GIZCLAW_E2E_DOCKER_PROJECT:?local Docker project is required}"
server="$(docker ps -q --filter "label=com.docker.compose.project=$project" --filter label=com.docker.compose.service=server)"
if [[ -z "$server" || "$server" == *$'\n'* ]]; then
  echo "expected one running Server in the local Docker project" >&2
  exit 1
fi
compose_files="$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' "$server")"
if [[ "$compose_files" != *"$repo_root/tests/gizclaw-e2e/docker/"* ]]; then
  echo "Docker Server does not belong to this checkout" >&2
  exit 1
fi

files=()
for file in "$@"; do
  case "$file" in
    "$repo_root"/*) files+=("/src/${file#"$repo_root"/}") ;;
    *) echo "Giztest document must be inside this checkout: $file" >&2; exit 2 ;;
  esac
done
environment=(-e GIZCLAW_TEST_ENDPOINT=edge:9821)
while IFS= read -r name; do
  case "$name" in
    GIZCLAW_TEST_ENDPOINT) ;;
    GIZCLAW_TEST_* | GIZCLAW_AUDIO_SAMPLE_TEXT | GIZCLAW_AUDIO_SAMPLE_VOICE | LATENCY_TONE)
      environment+=(-e "$name") ;;
  esac
done < <(compgen -e)

deadline="${GIZCLAW_E2E_CHAT_DEADLINE_SECONDS:-2700}"
if [[ ! "$deadline" =~ ^[1-9][0-9]*$ ]]; then
  echo "Giztest phase deadline must be a positive number of seconds" >&2
  exit 2
fi
temporary="$(mktemp "${report}.XXXXXX")"
pid_file="/tmp/gizclaw-standard-runner-$$.pid"
container_report="/tmp/gizclaw-standard-giztest-$$.json"
docker_client_pid=""
cleanup() {
  docker exec "$server" sh -c '
    if [ -r "$1" ]; then
      IFS= read -r runner_pid < "$1"
      case "$runner_pid" in
        "" | *[!0-9]* | 0 | 1) ;;
        *)
          if [ -r "/proc/$runner_pid/cmdline" ]; then
            runner_command="$(tr "\000" " " < "/proc/$runner_pid/cmdline")"
            case "$runner_command" in
              *"/gizclaw test run "*) kill -TERM "$runner_pid" 2>/dev/null || true ;;
            esac
          fi
          ;;
      esac
      rm -f "$1"
    fi
  ' sh "$pid_file" >/dev/null 2>&1 || true
  if [[ -n "$docker_client_pid" ]]; then
    kill -TERM "$docker_client_pid" >/dev/null 2>&1 || true
    wait "$docker_client_pid" >/dev/null 2>&1 || true
  fi
  rm -f "$temporary"
}
trap 'cleanup' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
status=0
echo "==> Go Giztest caller: Linux Docker, endpoint=edge:9821, parallel=10"
docker exec "${environment[@]}" -w /src "$server" \
  timeout --signal=TERM --kill-after=10s "${deadline}s" \
  sh -c 'echo $$ > "$1"; shift; exec "$@"' sh "$pid_file" \
  /src/tests/gizclaw-e2e/testdata/bin/gizclaw test run "${files[@]}" \
  --parallel 10 --report "$container_report" &
docker_client_pid=$!
wait "$docker_client_pid" || status=$?
docker_client_pid=""
docker cp "$server:$container_report" "$temporary"
mv "$temporary" "$report"
cleanup
trap - EXIT
exit "$status"
