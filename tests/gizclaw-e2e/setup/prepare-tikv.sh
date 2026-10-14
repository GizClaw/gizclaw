#!/usr/bin/env bash
# Build the fixture-only PD initializer and Raft proxy for the Docker host.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
arch=amd64
case "$(docker info --format '{{.Architecture}}')" in
  aarch64 | arm64) arch=arm64 ;;
  x86_64 | amd64) ;;
  *) echo 'unsupported Docker architecture for TiKV fixture' >&2; exit 2 ;;
esac
mkdir -p "$repo_root/tests/gizclaw-e2e/testdata/bin"
cd "$repo_root"
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o tests/gizclaw-e2e/testdata/bin/tikv-fixture-linux ./tests/gizclaw-e2e/cmd/tikv-fixture
GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -o tests/gizclaw-e2e/testdata/bin/tikv-replication-proxy-linux ./tests/store-e2e/tikv-replication-proxy
