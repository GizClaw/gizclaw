#!/usr/bin/env bash
# Local deterministic Edge/Server audio diagnostics; no provider credentials.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"
cd "$repo_dir"
python3 -I "$script_dir/setup/test_audio_netem_playback.py"
python3 -I "$script_dir/setup/test_audio_netem_evidence.py"
# This diagnostic must never follow a remote Docker context or DOCKER_HOST.
if [[ -n "${DOCKER_CONTEXT:-}" ]]; then
  docker_endpoint="$(docker context inspect "$DOCKER_CONTEXT" --format '{{.Endpoints.docker.Host}}')"
else
  docker_endpoint="${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}')}"
fi
case "$docker_endpoint" in
  unix://*) ;;
  *) printf 'Audio netem requires a local Unix Docker socket\n' >&2; exit 1 ;;
esac
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in arm64 | aarch64) arch=arm64 ;; esac
# Keep the build quota and Go parallelism within the local daemon's capacity.
build_cpus="$(docker info --format '{{.NCPU}}')"
if [[ ! "$build_cpus" =~ ^[1-9][0-9]*$ ]]; then
  printf 'Docker reported an invalid CPU count: %s\n' "$build_cpus" >&2
  exit 1
fi
if ((build_cpus > 4)); then build_cpus=4; fi
mkdir -p "$script_dir/.testbench"
run_dir="$(mktemp -d "$script_dir/.testbench/audio-netem-XXXXXX")"
export NETEM_ROOT="$repo_dir" NETEM_BIN="$run_dir/bin" NETEM_STATE="$run_dir/runtime" NETEM_REPORTS="$run_dir/reports"
export NETEM_BASE_IMAGE="${GIZCLAW_E2E_DOCKER_BASE_IMAGE:-gizclaw-go:linux-$arch-cn-base}"
export NETEM_TOOLS_IMAGE="gizclaw-audio-netem-tools:local"
NETEM_UID="$(id -u)"
NETEM_GID="$(id -g)"
export NETEM_UID NETEM_GID
mkdir -p "$NETEM_BIN" "$NETEM_STATE" "$NETEM_REPORTS"
project="gizclaw-audio-netem-$(date +%s)-$$"
export NETEM_PROJECT="$project"
compose=(docker compose --env-file /dev/null -p "$project" -f "$script_dir/docker/compose.audio-netem.yaml")
cleanup() {
  local status=$?
  trap - EXIT
  "${compose[@]}" logs --no-color > "$NETEM_REPORTS/containers.log" 2>&1 || true
  if "${compose[@]}" down --volumes --remove-orphans > "$NETEM_REPORTS/cleanup.log" 2>&1; then
    rm -rf "$NETEM_STATE"
  else
    printf 'Cleanup failed for Docker project %s; runtime state retained\n' "$project" >&2
    status=1
  fi
  docker rm -f "$project-build" >/dev/null 2>&1 || true
  printf 'Audio netem evidence: %s\n' "$NETEM_REPORTS"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
source "$script_dir/setup/docker-base.sh"
base_image=""
export GIZCLAW_E2E_DOCKER_BASE_IMAGE="$NETEM_BASE_IMAGE"
build_gizclaw_e2e_base "$repo_dir" "linux/$arch"
# Worktree sessions may have LFS installed without a persistent filter config.
# Supply it only to this command, preserving repository/global hook settings.
git -c filter.lfs.process='git-lfs filter-process' -c filter.lfs.clean='git-lfs clean -- %f' \
  -c filter.lfs.smudge='git-lfs smudge -- %f' -c filter.lfs.required=true \
  lfs pull --include="third_party/audio/prebuilt/*/linux-$arch/**,tests/gizclaw-e2e/testdata/audio/sfu-tone.ogg"
npm ci && npm run build:console
python3 -I "$script_dir/setup/prepare-audio-netem.py" "$repo_dir" "$NETEM_BIN"
docker build --build-arg "BASE_IMAGE=${GIZCLAW_NETEM_TOOLS_BASE:-gizcr.gizclaw.com/docker.io/library/alpine:3.22}" \
  -f "$script_dir/docker/Dockerfile.audio-netem" -t "$NETEM_TOOLS_IMAGE" "$script_dir/docker"
docker run --rm --name "$project-build" --cpus="$build_cpus" --memory=6g --entrypoint /bin/bash \
  -v "$repo_dir:/src:ro" -v "$NETEM_BIN:/out" \
  -v "gizclaw-netem-modcache:/gomod" -v "gizclaw-netem-buildcache:/cache" \
  -e GOMODCACHE=/gomod -e GOCACHE=/cache -e GOMAXPROCS="$build_cpus" -e "GOFLAGS=-p=$build_cpus -buildvcs=false" \
  -e "GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE=${GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE:-0}" \
  "$NETEM_BASE_IMAGE" -lc 'cd /src \
    && if [[ -f /out/sctp-source/go.mod ]]; then \
         cp go.mod /out/qualification.mod; cp go.sum /out/qualification.sum; \
         go mod edit -modfile=/out/qualification.mod -replace=github.com/pion/sctp=/out/sctp-source; \
         export GOFLAGS="$GOFLAGS -modfile=/out/qualification.mod"; \
       elif [[ "$GIZCLAW_NETEM_SCTP_DEFER_INITIAL_PROBE" == 1 ]]; then \
         cp go.mod /out/diagnostic.mod; cp go.sum /out/diagnostic.sum; \
         go mod edit -modfile=/out/diagnostic.mod -replace=github.com/pion/sctp=/out/sctp-probe; \
         export GOFLAGS="$GOFLAGS -modfile=/out/diagnostic.mod"; \
       fi \
    && go build -overlay /out/overlay.json -o /out/gizclaw ./cmd/gizclaw \
    && go build -o /out/monitor-seed ./tests/gizclaw-e2e/cmd/multiserver-seed \
    && go build -o /out/monitor-fixture ./tests/gizclaw-e2e/cmd/monitor-fixture \
    && go build -o /out/log-sink ./tests/gizclaw-e2e/cmd/log-sink \
    && go test -overlay /out/overlay.json -tags gizclaw_e2e -c -o /out/netem.test ./tests/gizclaw-e2e/go/multiserver'
docker run --rm --user "$NETEM_UID:$NETEM_GID" --workdir /src -v "$repo_dir/tests:/src/tests:ro" -v "$NETEM_BIN:/binaries:ro" \
  -v "$NETEM_STATE:/state" --entrypoint /binaries/monitor-fixture "$NETEM_BASE_IMAGE" -init /state
"${compose[@]}" up -d --wait server edge edge-netem server-netem test
"${compose[@]}" run --rm seed > "$NETEM_REPORTS/seed.log" 2>&1
python3 -I "$script_dir/setup/run_audio_netem.py" "$run_dir"
