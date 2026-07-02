#!/usr/bin/env bash
set -euo pipefail

repo_root="/src"
cd "$repo_root/apps/wails/frontend"

exec npm run dev -- --host 0.0.0.0 --port 4191
