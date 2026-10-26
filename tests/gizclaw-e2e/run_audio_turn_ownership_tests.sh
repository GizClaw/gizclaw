#!/usr/bin/env bash
# Linux Docker regressions for audio ownership and runtime retirement.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/audio-turn-ownership-XXXXXX")"
reports="$run_dir/reports"
mkdir -p "$reports"
project="gizclaw-audio-turn-ownership-$$"
network="$project"
container="$project-runner"
cache="gizclaw-audio-turn-ownership-buildcache"
run_user="$(id -u):$(id -g)"
cleanup() {
  local status=$?
  docker container rm -f "$container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  echo "Audio turn ownership Docker evidence: $reports"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
base="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
if ! docker image inspect "$base" >/dev/null 2>&1; then
  docker build -f "$repo_root/build/gizclaw/Dockerfile.cn.base" -t "$base" "$repo_root/build"
fi
(cd "$repo_root" && npm ci && npm run build:console && go mod download)
python3 - "$repo_root" "$reports/manifest.json" "$base" <<'PY'
import hashlib
import json
import pathlib
import subprocess
import sys

root, output = map(pathlib.Path, sys.argv[1:3])
base = json.loads(subprocess.check_output(["docker", "image", "inspect", sys.argv[3]], text=True))[0]
paths = subprocess.check_output(["git", "-C", str(root), "ls-files", "cmd/internal/commands/giztest/peer_stream*.go", "pkgs/genx/agentkit/audiodock/*.go", "pkgs/genx/transformers/eino/*.go", "pkgs/genx/transformers/doubaoasr/*.go", "pkgs/genx/transformers/doubaoast/*.go", "pkgs/genx/transformers/asttranslate/*.go", "pkgs/gizclaw/services/runtime/agenthost/*.go", "pkgs/giznet/giztunnel/*.go"], text=True).splitlines()
paths += ["cmd/internal/server/audio_turn_ownership_giztest_test.go", "tests/gizclaw-e2e/testdata/audio-turn-ownership/rounds.giztest.yaml"]
paths += ["cmd/internal/server/native_audio_history_giztest_test.go", "cmd/internal/server/podcast_giztest_test.go", "tests/gizclaw-e2e/testdata/continuous-podcast/eino.json", "tests/gizclaw-e2e/testdata/continuous-podcast/continuous.giztest.yaml"]
paths += ["go.mod", "go.sum", "tests/gizclaw-e2e/run_audio_turn_ownership_tests.sh", "tests/gizclaw-e2e/testdata/native-audio-history/resources.json"]
output.write_text(json.dumps({
    "revision": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
    "tracked_worktree_dirty": bool(subprocess.check_output(["git", "-C", str(root), "status", "--porcelain", "--untracked-files=no"], text=True)),
    "docker_image": base["Id"],
    "platform": base["Os"] + "/" + base["Architecture"],
    "provider_boundary": "local HTTP and SAUC WebSocket protocol fixtures; production ASR/model/TTS adapters",
    "transport": "real Server, Edge and WebRTC over a Docker-internal network",
    "sha256": {p: hashlib.sha256((root / p).read_bytes()).hexdigest() for p in paths},
}, indent=2) + "\n")
PY
docker network create --internal "$network" >/dev/null
docker run --rm --network none --entrypoint /bin/sh -v "$cache:/gocache" \
  "$base" -c "chown $run_user /gocache"
docker run --rm --user "$run_user" --name "$container" --network "$network" --entrypoint /bin/bash \
  -v "$repo_root:/src:ro" -v "$reports:/reports" \
  -v "$(go env GOMODCACHE):/gomodcache:ro" -v "$cache:/gocache" \
  -e HOME=/tmp -e GOPATH=/tmp/gopath -e GOMODCACHE=/gomodcache -e GOCACHE=/gocache \
  -e GOPROXY=off -e GOTOOLCHAIN=local -e GOFLAGS=-mod=readonly \
  -e HTTP_PROXY= -e HTTPS_PROXY= -e ALL_PROXY= \
  -e http_proxy= -e https_proxy= -e all_proxy= \
  -e GIZCLAW_TURN_OWNERSHIP_EVIDENCE=/reports/transport \
  -e GIZCLAW_NATIVE_HISTORY_EVIDENCE=/reports/native-history \
  -e GIZCLAW_TEST_PODCAST_GIZTEST_EVIDENCE=/reports/podcast \
  "$base" -lc 'set -euo pipefail
    cd /src
    go version > /reports/go-version.txt
    go test -race -json ./cmd/internal/commands/giztest ./pkgs/genx/agentkit/audiodock ./pkgs/genx/transformers/eino ./pkgs/genx/transformers/doubaoasr ./pkgs/genx/transformers/doubaoast ./pkgs/genx/transformers/asttranslate ./pkgs/gizclaw/services/runtime/agenthost ./pkgs/giznet/giztunnel -count=1 -timeout=8m > /reports/contracts.jsonl
    go test -race -json ./cmd/internal/server -run "^Test(AudioTurnOwnership|NativeAudioHistory|ContinuousPodcast)Giztest$" -count=1 -timeout=5m > /reports/transport.jsonl'
python3 - "$reports" <<'PY'
import collections
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / "manifest.json").read_text())
source = root.parents[4]
for name, expected in manifest["sha256"].items():
    if hashlib.sha256((source / name).read_bytes()).hexdigest() != expected:
        raise SystemExit(f"source changed during Docker qualification: {name}")
required = {
    "contracts": {"TestPeerStreamFirstResponseRejectsEarlierPTTOutput", "TestPeerStreamResponseHistorySurvivesCloseAndReopen", "TestPeerStreamConsecutiveTurnsKeepResponseOwnership", "TestPeerStreamRearmRejectsKnownResponseTerminalErrors", "TestPeerStreamPTTInterruptPreservesAdmittedTerminal", "TestOutputEpochRetainsProducerInput", "TestDockPreservesDelayedProducerEpoch", "TestTextAgentWaitsForASRTerminalAcrossControlEOSOrder", "TestPushToTalkReplacementClosesPreviousHistoryAndTranscript", "TestRuntimeStopClosesInputBeforeWaitingForOutput", "TestWorkspaceQuiescenceClosesInputBeforeWaitingForOutput", "TestOpusOverflowKeepsLogicalControlAndServices", "TestDirectPacketOverflowRetainsItsFailureBoundary", "TestRecoveringSessionCompletedSendWinsSessionClose", "TestRecoveringSessionSendAfterCompletionReturnsClosedPipe"},
    "transport": {"TestAudioTurnOwnershipGiztest/push-to-talk/retained=false", "TestAudioTurnOwnershipGiztest/realtime/retained=false", "TestAudioTurnOwnershipGiztest/realtime/retained=true", "TestAudioTurnOwnershipGiztest/push-to-talk/retained=false/advance=input_sent", "TestNativeAudioHistoryGiztest", "TestContinuousPodcastGiztest"},
}
required["contracts"].update({
    "TestPTTPreReleaseCountExcludesKnownResponsesAndCountsEachNewID",
    "TestCaptureOutputReleasesBeforeProviderReadsEOS",
    "TestCaptureOutputPreparesExternalTTSWhileUploaderIsBlocked",
    "TestCaptureOutputDropsReusedInputIDEpoch",
    "TestCaptureOutputDefersHistoryAndDeliveryObservation",
    "TestPrepareSpeechOutputRealtimeEndsWithoutNextInputPacket",
    "TestPrepareSpeechOutputKeepsTranscriptOpenAcrossCaptureControlEOS",
    "TestHistoryInterruptSupersedesUndeliveredDurableTerminal",
    "TestInterruptibleOutputStreamsAvailableTextAndAudioWithoutDelay",
    "TestTransformerPreservesInputEpochAcrossReusedStreamID",
})
required["contracts"].update({
    "TestTransformerInterimDoesNotReplayCommittedResults/repeated_committed_snapshot",
    "TestTransformerInterimDoesNotReplayCommittedResults/text_populated_after_committed_utterance",
    "TestTransformerInterimDoesNotReplayCommittedResults/same_words_in_a_new_timed_utterance",
    "TestTransformerInterimDoesNotReplayCommittedResults/new_interim_repeats_earlier_words",
    "TestTransformerInterimDoesNotReplayCommittedResults/cumulative_committed_utterances_with_new_interim_text",
})
required["transport"].add("TestAudioTurnOwnershipGiztest/realtime/repeated-committed-asr/retained=true")
summary = {}
for lane, tests in required.items():
    events = [json.loads(line) for line in (root / (lane + ".jsonl")).read_text().splitlines()]
    actions = collections.Counter(event["Action"] for event in events)
    passed = {event.get("Test") for event in events if event["Action"] == "pass"}
    if actions["fail"] or not tests.issubset(passed):
        raise SystemExit(f"{lane}: missing passing tests {sorted(tests - passed)}; failures={actions['fail']}")
    summary[lane] = {"required_passes": sorted(tests), "test_pass_events": sum(event["Action"] == "pass" and "Test" in event for event in events)}
(root / "summary.json").write_text(json.dumps(summary, indent=2) + "\n")
print(json.dumps(summary, indent=2))
PY
