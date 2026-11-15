#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
image="${DOUBAO_PTT_TEST_IMAGE:-gizcr.gizclaw.com/docker.io/library/golang:1.26.8}"
cache_args=()
if [[ -n "${DOUBAO_PTT_GOMODCACHE:-}" ]]; then
  cache_args+=(--mount "type=bind,source=${DOUBAO_PTT_GOMODCACHE},target=/gomod,readonly" --env GOMODCACHE=/gomod)
fi

# Select only the untagged local PTT cases; do not source cloud credentials.
# The protocol fixture listens only on loopback inside this disposable container.
# No credentials, host ports, existing containers or persistent volumes are used.
exec docker run --rm --name "gizclaw-doubao-ptt-$$" \
  --label org.gizclaw.test=doubao-ptt \
  --mount "type=bind,source=${repo_root},target=/src,readonly" \
  ${cache_args[@]+"${cache_args[@]}"} \
  --workdir /src --env CGO_ENABLED=0 --env GOTOOLCHAIN=local \
  --entrypoint go "${image}" \
  test ./tests/genx-e2e/transformer \
  -run '^TestPTTWebSocket(Recovery|CaptureFailure)$' -count=1 -v -timeout=30s
