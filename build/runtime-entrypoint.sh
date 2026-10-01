#!/bin/sh
# Own the container workspace for the lifetime of the foreground Server.
set -eu
if [ "$#" -eq 3 ] && [ "$1" = serve ] && [ "$2" = --force ] && [ "$3" = /var/lib/gizclaw ]; then
  # A container's PID namespace can reuse the PID recorded before a crash.
  # Keep the lock FD across exec and remove that PID only after claiming ownership.
  exec 9>/var/lib/gizclaw/.container.lock
  if ! flock --nonblock 9; then
    echo 'GizClaw container workspace already owned by another Server' >&2
    exit 1
  fi
  rm -f /var/lib/gizclaw/serve.pid
fi
exec /usr/bin/gizclaw "$@"
