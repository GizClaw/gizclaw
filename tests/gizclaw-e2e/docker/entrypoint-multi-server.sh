#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
workspace_dir="/var/lib/gizclaw"
config_file="$workspace_dir/config.yaml"
sfu_dir="$workspace_dir/sfu"

: "${GIZCLAW_E2E_SERVER_PRIVATE_KEY:?missing GIZCLAW_E2E_SERVER_PRIVATE_KEY}"
: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
: "${GIZCLAW_E2E_REDIS_DSN:?missing GIZCLAW_E2E_REDIS_DSN}"
: "${GIZCLAW_E2E_ADMIN_PUBLIC_KEY:?missing GIZCLAW_E2E_ADMIN_PUBLIC_KEY}"
: "${GIZCLAW_E2E_LIVEKIT_URL:?missing GIZCLAW_E2E_LIVEKIT_URL}"
: "${GIZCLAW_E2E_LIVEKIT_HTTP_URL:?missing GIZCLAW_E2E_LIVEKIT_HTTP_URL}"
: "${GIZCLAW_E2E_LIVEKIT_API_KEY:?missing GIZCLAW_E2E_LIVEKIT_API_KEY}"
: "${GIZCLAW_E2E_LIVEKIT_API_SECRET:?missing GIZCLAW_E2E_LIVEKIT_API_SECRET}"

# Compose already gates on the livekit healthcheck; this wait covers a
# restarted Server whose dependency ordering no longer applies.
livekit_deadline=$((SECONDS + 120))
until curl -fsS -o /dev/null "$GIZCLAW_E2E_LIVEKIT_HTTP_URL/"; do
  if ((SECONDS >= livekit_deadline)); then
    echo "LiveKit signaling did not become ready at $GIZCLAW_E2E_LIVEKIT_HTTP_URL" >&2
    exit 1
  fi
  sleep 1
done

# Store layout: peers, friends and friend-groups are the shared Social KV in
# Redis; runtime-profiles stay in the template's Server-local memory store and
# workspaces/workflows share the Server-local gameplay SQLite file, so a
# Social Workspace retirement and the gameplay reward fence exercise one
# single-connection SQLite handle exactly as a real deployment would.
mkdir -p "$workspace_dir/data" "$sfu_dir"
umask 077
printf '%s\n' "$GIZCLAW_E2E_LIVEKIT_API_KEY" >"$sfu_dir/api_key"
printf '%s\n' "$GIZCLAW_E2E_LIVEKIT_API_SECRET" >"$sfu_dir/api_secret"
umask 022
cp "$repo_root/tests/gizclaw-e2e/testdata/server-workspace/config.yaml.template" "$config_file"

GIZCLAW_E2E_SERVER_PRIVATE_KEY="$GIZCLAW_E2E_SERVER_PRIVATE_KEY" \
GIZCLAW_E2E_SERVER_ENDPOINT="$GIZCLAW_E2E_SERVER_ENDPOINT" \
GIZCLAW_E2E_REDIS_DSN="$GIZCLAW_E2E_REDIS_DSN" \
GIZCLAW_E2E_ADMIN_PUBLIC_KEY="$GIZCLAW_E2E_ADMIN_PUBLIC_KEY" \
GIZCLAW_E2E_LIVEKIT_URL="$GIZCLAW_E2E_LIVEKIT_URL" \
GIZCLAW_E2E_SFU_DIR="$sfu_dir" \
perl -0pi -e '
  s/private-key: 287thuEga1hbBYjM7sxrY3eMoPAqpB3nJ81S8CEuNHKu/private-key: $ENV{GIZCLAW_E2E_SERVER_PRIVATE_KEY}/;
  s/endpoint: \$\{GIZCLAW_E2E_SERVER_ENDPOINT\}/endpoint: $ENV{GIZCLAW_E2E_SERVER_ENDPOINT}/;
  s/admin-public-key: "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"/admin-public-key: "$ENV{GIZCLAW_E2E_ADMIN_PUBLIC_KEY}"/;
  s/ice-servers:\n(?:  .*\n)+?(?=edge-nodes:)/""/e;
  s/storage:\n/storage:\n  shared-redis:\n    kind: redis\n    url: $ENV{GIZCLAW_E2E_REDIS_DSN}\n/;
  s/(  peers:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  api-keys:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  friends:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  friend-groups:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  workspaces:\n    kind: keyvalue\n    storage:) memory/$1 gameplay-db/;
  s/(  workflows:\n    kind: keyvalue\n    storage:) memory/$1 gameplay-db/;
  s/^services:\n/services:\n  sfu:\n    url: $ENV{GIZCLAW_E2E_LIVEKIT_URL}\n    api_key_file: $ENV{GIZCLAW_E2E_SFU_DIR}\/api_key\n    api_secret_file: $ENV{GIZCLAW_E2E_SFU_DIR}\/api_secret\n/m;
' "$config_file"

if ! grep -q '^  sfu:$' "$config_file"; then
  echo "services.sfu was not written into $config_file" >&2
  exit 1
fi

exec /usr/local/bin/gizclaw serve --force "$workspace_dir"
