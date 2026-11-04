#!/usr/bin/env bash
# Run Mem0 contracts and LoCoMo fixture regressions without external services.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
cd "$repo_root"

export GOFLAGS=-mod=readonly
go test -count=1 ./sdk/go/mem0 ./pkgs/store/memory/mem0 ./tests/evaluation/locomo/cmd/fixturegen
go generate ./sdk/go/mem0
git diff --exit-code -- sdk/go/mem0/generated.go
go test -count=1 -tags gizclaw_locomo_e2e \
  -run '^Test(Dataset|Score|Adversarial|Aggregate|RunBenchmark|AwaitObservation|RunQuestion|Matches|CloseStore|Preflight|NewLLM|Redaction|SessionObservations|Regression|TurnIngestion|VolcComparison|SDKLoadMetrics)' \
  ./tests/evaluation/locomo
