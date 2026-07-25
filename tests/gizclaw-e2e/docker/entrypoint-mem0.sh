#!/usr/bin/env bash
set -euo pipefail

env_file="/run/gizclaw-e2e.env"
if [[ ! -f "$env_file" ]]; then
  echo "missing mounted GizClaw E2E credential file" >&2
  exit 2
fi

exec uvicorn mem0_server:app --host 0.0.0.0 --port 8000
