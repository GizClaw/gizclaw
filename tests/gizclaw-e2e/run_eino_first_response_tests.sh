#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
setup_dir="$script_dir/setup"
env_file="$script_dir/.env"
gizclaw_binary="$script_dir/testdata/bin/gizclaw"
artifact_dir="${GIZCLAW_E2E_EINO_FIRST_RESPONSE_ARTIFACT_DIR:-$script_dir/testdata/eino-first-response}"
docker_env_path="$(mktemp "${TMPDIR:-/tmp}/gizclaw-eino-first-response.XXXXXX")"
rm -f "$docker_env_path"
export GIZCLAW_E2E_DOCKER_ENV="$docker_env_path"
project_user="$(printf '%s' "${USER:-user}" | tr -cd '[:alnum:]' | tr '[:upper:]' '[:lower:]')"
export GIZCLAW_E2E_DOCKER_PROJECT="gizclaw-eino-first-response-${project_user:-user}-$$"
stack_started=0

# shellcheck source=setup/credentials.sh
# shellcheck disable=SC1091
source "$setup_dir/credentials.sh"
require_gizclaw_e2e_credentials "$env_file"

collect_failure_logs() {
	if [[ "$stack_started" != "1" ]]; then
		return
	fi
	local compose_file="${GIZCLAW_E2E_DOCKER_COMPOSE_FILE:-$script_dir/docker/docker-compose.yaml}"
	local -a compose_args=(-f "$compose_file")
	if [[ -n "${GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY:-}" ]]; then
		compose_args+=(-f "$GIZCLAW_E2E_DOCKER_COMPOSE_OVERLAY")
	fi
	docker compose -p "$GIZCLAW_E2E_DOCKER_PROJECT" "${compose_args[@]}" logs \
		--no-color --tail=200 server edge edge2 2>&1 |
		python3 "$setup_dir/redact_diagnostics.py" >&2 || true
}

cleanup() {
	local status=$?
	if ((status != 0)); then
		collect_failure_logs
	fi
	if [[ "$stack_started" == "1" || -f "$docker_env_path" ]]; then
		if ! bash "$setup_dir/docker-compose-down.sh" && ((status == 0)); then
			status=1
		fi
	fi
	rm -f "$docker_env_path" "$gizclaw_binary"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ "${GIZCLAW_E2E_ALLOW_DIRTY:-0}" != "1" ]] &&
	[[ -n "$(git -C "$repo_root" status --porcelain --untracked-files=no)" ]]; then
	echo "Eino first-response qualification requires a clean tracked worktree; set GIZCLAW_E2E_ALLOW_DIRTY=1 for exploratory runs" >&2
	exit 2
fi

mkdir -p "$script_dir/testdata/bin" "$artifact_dir"
for target in server edge; do
	for parallel in 1 8; do
		rm -f \
			"$artifact_dir/${target}-text-p${parallel}.json" \
			"$artifact_dir/${target}-push-to-talk-p${parallel}.json" \
			"$artifact_dir/${target}-realtime-p${parallel}.json"
	done
	rm -f \
		"$artifact_dir/${target}-push-to-talk-roundtrip.json" \
		"$artifact_dir/${target}-realtime-roundtrip.json"
done
rm -f "$artifact_dir/manifest.json" "$artifact_dir/manifest.json.tmp"

echo "==> build Eino first-response qualification CLI"
(cd "$repo_root" && go build -o "$gizclaw_binary" ./cmd/gizclaw)

echo "==> start isolated Docker e2e stack project=$GIZCLAW_E2E_DOCKER_PROJECT"
bash "$setup_dir/docker-compose-up.sh"
stack_started=1
set -a
# shellcheck disable=SC1090
source "$docker_env_path"
set +a

text_case="$script_dir/giztest/benchmark.eino-concurrency-assistant.concurrency-10.giztest.yaml"
push_to_talk_case="$script_dir/giztest/benchmark.eino-concurrency-assistant.push-to-talk-concurrency-10.giztest.yaml"
realtime_case="$script_dir/giztest/benchmark.eino-concurrency-assistant.realtime-concurrency-10.giztest.yaml"
push_to_talk_roundtrip_case="$script_dir/giztest/eino-concurrency-assistant.push-to-talk-roundtrip.giztest.yaml"
realtime_roundtrip_case="$script_dir/giztest/eino-concurrency-assistant.realtime-roundtrip.giztest.yaml"
run_status=0

run_case() {
	local target="$1"
	local mode="$2"
	local parallel="$3"
	local endpoint="$4"
	local path="$5"
	local report="$artifact_dir/${target}-${mode}-p${parallel}.json"
	echo "==> qualify target=$target mode=$mode parallel=$parallel"
	if ! GIZCLAW_TEST_ENDPOINT="$endpoint" "$gizclaw_binary" test run \
		--parallel "$parallel" --output "$report" "$path"; then
		run_status=1
	fi
}

run_roundtrip() {
	local target="$1"
	local mode="$2"
	local endpoint="$3"
	local path="$4"
	local report="$artifact_dir/${target}-${mode}-roundtrip.json"
	echo "==> verify terminal roundtrip target=$target mode=$mode"
	if ! GIZCLAW_TEST_ENDPOINT="$endpoint" "$gizclaw_binary" test run \
		--parallel 1 --output "$report" "$path"; then
		run_status=1
	fi
}

for target in server edge; do
	case "$target" in
	server) endpoint="$GIZCLAW_E2E_SERVER_ENDPOINT" ;;
	edge) endpoint="$GIZCLAW_E2E_EDGE_ENDPOINT" ;;
	esac
	for parallel in 1 8; do
		run_case "$target" text "$parallel" "$endpoint" "$text_case"
		run_case "$target" push-to-talk "$parallel" "$endpoint" "$push_to_talk_case"
		run_case "$target" realtime "$parallel" "$endpoint" "$realtime_case"
	done
	run_roundtrip "$target" push-to-talk "$endpoint" "$push_to_talk_roundtrip_case"
	run_roundtrip "$target" realtime "$endpoint" "$realtime_roundtrip_case"
done

python3 - "$repo_root" "$artifact_dir" <<'PY'
import hashlib
import json
import os
import subprocess
import sys

repo_root, artifact_dir = sys.argv[1:]
case_specs = []
for target in ("server", "edge"):
    for parallel in (1, 8):
        case_specs.append((f"{target}-text-p{parallel}", 10, False, True, False))
        case_specs.append((f"{target}-push-to-talk-p{parallel}", 10, True, True, True))
        case_specs.append((f"{target}-realtime-p{parallel}", 10, True, True, True))
    case_specs.append((f"{target}-push-to-talk-roundtrip", 1, False, False, False))
    case_specs.append((f"{target}-realtime-roundtrip", 1, False, False, False))

def file_sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for block in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()

def git_output(*args):
    return subprocess.check_output(("git", "-C", repo_root, *args), text=True).strip()

model_path = os.path.join(
    repo_root,
    "tests/gizclaw-e2e/testdata/resources/03-models/04-doubao-mini-chat.yaml",
)
profile_path = os.path.join(
    repo_root,
    "tests/gizclaw-e2e/testdata/resources/09-giztest/01-runtime-profile.yaml",
)
with open(model_path, encoding="utf-8") as handle:
    model_source = handle.read()
with open(profile_path, encoding="utf-8") as handle:
    profile_source = handle.read()
for required in (
    "metadata:\n  id: doubao-mini-chat\n",
    "    id: volc-ark\n",
    "    upstream_model: doubao-seed-2-0-mini-260428\n",
):
    if required not in model_source:
        raise SystemExit(f"qualified chat Model contract is missing {required.strip()!r}")
for required in (
    "llm: {resource_id: doubao-mini-chat,",
    "asr: {resource_id: volc-bigasr-sauc,",
    "narrator: {resource_id:",
):
    if required not in profile_source:
        raise SystemExit(f"qualified RuntimeProfile contract is missing {required!r}")

summary = {
    "version": "gizclaw.eino-first-response/v1",
    "git_revision": git_output("rev-parse", "HEAD"),
    "worktree_dirty": bool(git_output("status", "--porcelain", "--untracked-files=no")),
    "qualified_resources": {
        "chat_model_resource_id": "doubao-mini-chat",
        "chat_model_upstream": "doubao-seed-2-0-mini-260428",
        "chat_model_tenant": "volc-ark",
        "asr_model_resource_id": "volc-bigasr-sauc",
        "voice_alias": "narrator",
    },
    "resource_sha256": {},
    "cases": {},
}
resource_paths = (
    "tests/gizclaw-e2e/testdata/resources/03-models/04-doubao-mini-chat.yaml",
    "tests/gizclaw-e2e/testdata/resources/09-giztest/01-runtime-profile.yaml",
    "tests/gizclaw-e2e/giztest/benchmark.eino-concurrency-assistant.concurrency-10.giztest.yaml",
    "tests/gizclaw-e2e/giztest/benchmark.eino-concurrency-assistant.push-to-talk-concurrency-10.giztest.yaml",
    "tests/gizclaw-e2e/giztest/benchmark.eino-concurrency-assistant.realtime-concurrency-10.giztest.yaml",
    "tests/gizclaw-e2e/giztest/eino-concurrency-assistant.push-to-talk-roundtrip.giztest.yaml",
)
for relative_path in resource_paths:
    summary["resource_sha256"][relative_path] = file_sha256(os.path.join(repo_root, relative_path))

errors = []
for name, expected_tasks, require_transcript, require_text, require_audio in case_specs:
    path = os.path.join(artifact_dir, f"{name}.json")
    if not os.path.isfile(path):
        errors.append(f"{name}: missing report")
        continue
    with open(path, encoding="utf-8") as handle:
        report = json.load(handle)
    tasks = report.get("tasks", [])
    text_values = []
    audio_values = []
    transcript_values = []
    cleanup_steps = 0
    if report.get("status") != "passed":
        errors.append(f"{name}: report status={report.get('status')}")
    if len(tasks) != expected_tasks:
        errors.append(f"{name}: tasks={len(tasks)} expected={expected_tasks}")
    for task in tasks:
        if task.get("status") != "passed":
            errors.append(f"{name}/{task.get('task_id')}: status={task.get('status')}")
        cleanup = task.get("cleanup", [])
        cleanup_steps += len(cleanup)
        if len(cleanup) != 3 or any(step.get("status") != "passed" for step in cleanup):
            errors.append(f"{name}/{task.get('task_id')}: cleanup did not pass all three steps")
        stream = next((step for step in task.get("steps", []) if step.get("operation") == "peer_stream"), None)
        if stream is None:
            errors.append(f"{name}/{task.get('task_id')}: missing peer_stream evidence")
            continue
        evidence = stream.get("evidence", {})
        if require_transcript:
            value = evidence.get("first_transcript_ms")
            if not isinstance(value, (int, float)) or value < 1 or value > 700:
                errors.append(f"{name}/{task.get('task_id')}: first_transcript_ms={value}")
            else:
                transcript_values.append(value)
        if require_text:
            value = evidence.get("first_text_ms")
            if not isinstance(value, (int, float)) or value > 2000:
                errors.append(f"{name}/{task.get('task_id')}: first_text_ms={value}")
            else:
                text_values.append(value)
        if require_audio:
            value = evidence.get("first_audio_ms")
            if not isinstance(value, (int, float)) or value > 3000:
                errors.append(f"{name}/{task.get('task_id')}: first_audio_ms={value}")
            else:
                audio_values.append(value)
    summary["cases"][name] = {
        "report_sha256": file_sha256(path),
        "status": report.get("status"),
        "tasks": len(tasks),
        "cleanup_steps": cleanup_steps,
        "max_first_transcript_ms": max(transcript_values) if transcript_values else None,
        "max_first_text_ms": max(text_values) if text_values else None,
        "max_first_audio_ms": max(audio_values) if audio_values else None,
    }

output = os.path.join(artifact_dir, "manifest.json")
temporary = output + ".tmp"
with open(temporary, "w", encoding="utf-8") as handle:
    json.dump(summary, handle, indent=2, sort_keys=True)
    handle.write("\n")
os.replace(temporary, output)
if errors:
    raise SystemExit("Eino first-response qualification failed:\n- " + "\n- ".join(errors))
PY

if ((run_status != 0)); then
	echo "one or more Eino first-response commands failed" >&2
	exit 1
fi

echo "==> Eino first-response qualification passed manifest=$artifact_dir/manifest.json"
