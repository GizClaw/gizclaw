#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../.." && pwd)"
project_name="${GIZCLAW_LOCOMO_E2E_DOCKER_PROJECT:-gizclaw-mem0-regression}"
compose=(docker compose --project-name "$project_name" --file "$script_dir/docker-compose.yaml")
cleanup() { "${compose[@]}" down --volumes --remove-orphans; }
trap cleanup EXIT

cd "$repo_root"
go test -count=1 ./sdk/go/mem0 ./pkgs/store/memory/mem0 ./tests/locomo-e2e/cmd/fixturegen
before_codegen="$(shasum -a 256 sdk/go/mem0/generated.go)"
go generate ./sdk/go/mem0
test "$before_codegen" = "$(shasum -a 256 sdk/go/mem0/generated.go)"
go test -count=1 -tags gizclaw_locomo_e2e -run '^Test(Dataset|Score|Adversarial|Aggregate|RunBenchmark|AwaitObservation|RunQuestion|Matches|CloseStore|Preflight|NewLLM|Redaction|SessionObservations|Regression|TurnIngestion|VolcComparison|SDKLoadMetrics)' ./tests/locomo-e2e
platform="$(docker version --format '{{.Server.Os}}/{{.Server.Arch}}')"
export GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE="gizclaw-mem0:${platform//\//-}-base"
TARGET=base PLATFORM="$platform" BASE_IMAGE="$GIZCLAW_LOCOMO_E2E_MEM0_BASE_IMAGE" "$repo_root/build/build-mem0.sh"
"${compose[@]}" build mem0_pgvector
"${compose[@]}" up --detach --wait pgvector
"${compose[@]}" run --rm --no-deps --entrypoint python mem0_pgvector -m unittest -v mem0_pgvector_regression_test.py
