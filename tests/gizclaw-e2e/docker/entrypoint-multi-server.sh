#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
workspace_dir="/var/lib/gizclaw"
config_file="$workspace_dir/config.yaml"

: "${GIZCLAW_E2E_SERVER_PRIVATE_KEY:?missing GIZCLAW_E2E_SERVER_PRIVATE_KEY}"
: "${GIZCLAW_E2E_SERVER_ENDPOINT:?missing GIZCLAW_E2E_SERVER_ENDPOINT}"
: "${GIZCLAW_E2E_REDIS_DSN:?missing GIZCLAW_E2E_REDIS_DSN}"
: "${GIZCLAW_E2E_ADMIN_PUBLIC_KEY:?missing GIZCLAW_E2E_ADMIN_PUBLIC_KEY}"

mkdir -p "$workspace_dir/data"
cp "$repo_root/tests/gizclaw-e2e/testdata/server-workspace/config.yaml.template" "$config_file"

GIZCLAW_E2E_SERVER_PRIVATE_KEY="$GIZCLAW_E2E_SERVER_PRIVATE_KEY" \
GIZCLAW_E2E_SERVER_ENDPOINT="$GIZCLAW_E2E_SERVER_ENDPOINT" \
GIZCLAW_E2E_REDIS_DSN="$GIZCLAW_E2E_REDIS_DSN" \
GIZCLAW_E2E_ADMIN_PUBLIC_KEY="$GIZCLAW_E2E_ADMIN_PUBLIC_KEY" \
perl -0pi -e '
  s/private-key: 287thuEga1hbBYjM7sxrY3eMoPAqpB3nJ81S8CEuNHKu/private-key: $ENV{GIZCLAW_E2E_SERVER_PRIVATE_KEY}/;
  s/endpoint: \$\{GIZCLAW_E2E_SERVER_ENDPOINT\}/endpoint: $ENV{GIZCLAW_E2E_SERVER_ENDPOINT}/;
  s/admin-public-key: "6Ww6ANsXDCf91Yp7Tvi65hqpywjMmXqAoZDiq33kfCee"/admin-public-key: "$ENV{GIZCLAW_E2E_ADMIN_PUBLIC_KEY}"/;
  s/ice-servers:\n(?:  .*\n)+?(?=edge-nodes:)/""/e;
  s/storage:\n/storage:\n  shared-redis:\n    kind: redis\n    dsn: $ENV{GIZCLAW_E2E_REDIS_DSN}\n/;
  s/(  peers:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  friends:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  friend-groups:\n    kind: keyvalue\n    storage:) memory/$1 shared-redis/;
  s/(  runtime-profiles:\n    kind: keyvalue\n    storage:) memory/$1 gameplay-db/;
  s/(  workspaces:\n    kind: keyvalue\n    storage:) memory/$1 gameplay-db/;
  s/(  workflows:\n    kind: keyvalue\n    storage:) memory/$1 gameplay-db/;
' "$config_file"

exec /usr/local/bin/gizclaw serve --force "$workspace_dir"
