#!/usr/bin/env bash
set -euo pipefail

workspace_dir="/var/lib/gizclaw-edge"
config_file="$workspace_dir/config.yaml"

: "${GIZCLAW_E2E_EDGE_PRIVATE_KEY:?missing GIZCLAW_E2E_EDGE_PRIVATE_KEY}"
: "${GIZCLAW_E2E_EDGE_ENDPOINT:?missing GIZCLAW_E2E_EDGE_ENDPOINT}"
: "${GIZCLAW_E2E_UPSTREAM_A_ENDPOINT:?missing GIZCLAW_E2E_UPSTREAM_A_ENDPOINT}"
: "${GIZCLAW_E2E_UPSTREAM_A_PUBLIC_KEY:?missing GIZCLAW_E2E_UPSTREAM_A_PUBLIC_KEY}"
: "${GIZCLAW_E2E_UPSTREAM_B_ENDPOINT:?missing GIZCLAW_E2E_UPSTREAM_B_ENDPOINT}"
: "${GIZCLAW_E2E_UPSTREAM_B_PUBLIC_KEY:?missing GIZCLAW_E2E_UPSTREAM_B_PUBLIC_KEY}"

mkdir -p "$workspace_dir"
{
  echo "identity:"
  echo "  private-key: $GIZCLAW_E2E_EDGE_PRIVATE_KEY"
  echo "webrtc:"
  echo "  listen: 0.0.0.0:9821"
  echo "  endpoint: $GIZCLAW_E2E_EDGE_ENDPOINT"
  echo "upstreams:"
  echo "  - endpoint: $GIZCLAW_E2E_UPSTREAM_A_ENDPOINT"
  echo "    public-key: $GIZCLAW_E2E_UPSTREAM_A_PUBLIC_KEY"
  echo "  - endpoint: $GIZCLAW_E2E_UPSTREAM_B_ENDPOINT"
  echo "    public-key: $GIZCLAW_E2E_UPSTREAM_B_PUBLIC_KEY"
  echo "http:"
  echo "  listeners:"
  echo "    - listen: 0.0.0.0:9821"
  echo "gateway:"
  echo "  enabled: true"
} > "$config_file"

exec /usr/local/bin/gizclaw edge serve "$workspace_dir"
