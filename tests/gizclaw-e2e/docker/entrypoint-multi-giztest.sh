#!/usr/bin/env bash
# Runs Giztest scenario files against the multi-server stack. Every argument
# is a scenario path relative to /src. The provider-free tone fixture is
# exported as base64 so `type: audio` input variables can load it from the
# environment without TTS credentials.
set -euo pipefail

repo_root="/src"
report="${GIZCLAW_E2E_GIZTEST_REPORT:?missing GIZCLAW_E2E_GIZTEST_REPORT}"
# `full` keeps the raw step evidence (transcripts, relay text) in the report;
# the default `redacted` mode only records the operation.
evidence="${GIZCLAW_E2E_GIZTEST_EVIDENCE:-redacted}"
tone_fixture="$repo_root/tests/gizclaw-e2e/testdata/audio/sfu-tone.ogg"

: "${GIZCLAW_TEST_EDGE_A:?missing GIZCLAW_TEST_EDGE_A}"
: "${GIZCLAW_TEST_EDGE_B:?missing GIZCLAW_TEST_EDGE_B}"
: "${GIZCLAW_TEST_REGISTRATION_TOKEN_A:?missing GIZCLAW_TEST_REGISTRATION_TOKEN_A}"
: "${GIZCLAW_TEST_REGISTRATION_TOKEN_B:?missing GIZCLAW_TEST_REGISTRATION_TOKEN_B}"

if (($# == 0)); then
  echo "usage: entrypoint-multi-giztest.sh <scenario.giztest.yaml>..." >&2
  exit 2
fi

GIZCLAW_TEST_SFU_TONE_OGG_BASE64="$(base64 -w0 "$tone_fixture")"
export GIZCLAW_TEST_SFU_TONE_OGG_BASE64

mkdir -p "$(dirname "$report")"
cd "$repo_root"
exec /usr/local/bin/gizclaw test run "$@" --parallel 1 --evidence "$evidence" --output "$report"
