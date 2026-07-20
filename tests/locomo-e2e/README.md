# Manual LoCoMo memory evaluation

This directory contains a GizClaw-owned LoCoMo runner for production
`memory.Store` implementations. It does not use Flowcraft's LoCoMo evaluator,
and it is intentionally excluded from normal `go test ./...`,
`tests/gizclaw-e2e/run_tests.sh`, and required CI.

Every live test is one fully named provider + memory lane + extraction config.
The Go file that owns the test also owns its complete adapter config:

| Test | File | Meaning |
| --- | --- | --- |
| `TestLoCoMoFlowcraftBM25SinglePass` | `flowcraft_bm25_single_pass_test.go` | BBH lexical lane, single-pass extraction |
| `TestLoCoMoFlowcraftHybridSinglePass` | `flowcraft_hybrid_single_pass_test.go` | BBH lexical+vector lane, single-pass extraction |
| `TestLoCoMoFlowcraftHybridTwoPass` | `flowcraft_hybrid_two_pass_test.go` | BBH lexical+vector lane, two-pass extraction |
| `TestLoCoMoMem0PlatformDefault` | `mem0_platform_default_test.go` | managed Mem0 default project config |
| `TestLoCoMoMem0PlatformCustomInstructions` | `mem0_platform_custom_instructions_test.go` | separately provisioned managed project with custom instructions |
| `TestLoCoMoVolcAgentKitDefault` | `volc_agentkit_default_test.go` | Volcengine AgentKit Memory default project config |

Remote extraction config is project/deployment state. The harness does not
mutate it and never labels the same endpoint/project as two lanes. The custom
Mem0 test therefore requires its own endpoint/key plus a non-secret deployment
fingerprint.

## Dataset

`testdata/locomo10_smoke.jsonl` is a Git LFS object derived from the official
SNAP Research `locomo10.json`. It contains the first three sessions of
`conv-30` (58 turns) and six fixed questions covering temporal, single-hop,
and open-domain categories. The manifest records the exact upstream commit,
source checksum, subset IDs, transformation, and license. It is a contract
smoke dataset, not a leaderboard-equivalent reproduction of all LoCoMo data.

Run `git lfs pull` after cloning. The loader rejects an unresolved LFS pointer
instead of attempting to parse it as benchmark data.

## Run

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
# Fill the selected profile credentials. Doubao Seed 2.0 Lite is the default
# extraction and answer model.
bash tests/locomo-e2e/run_tests.sh
```

The script accepts one explicit live test name and applies a 30-minute default
whole-test timeout plus per-session and per-question timeouts.
The Go runner groups source turns by their official session, writes each session
through `memory.Store.Observe` with the official speaker and session time,
recalls for every question, asks Doubao to answer from the returned facts, and
computes deterministic EM/F1 and evidence-hit metrics locally. No Python package
or official LoCoMo Go library is required.
The default quality gate rejects a run with aggregate F1 below `0.05`, and
rejects evidence-aware stores with an evidence hit rate below `0.50`; these
thresholds can be raised for a lane through `.env`. The committed dataset also
requires at least one materialized fact per selected session, and the runner
rejects any provider that violates the `memory.Store` descending-score recall
contract.

Each run uses unique opaque conversation scopes and a local sandbox. Remote
cleanup is deliberately conservative: the harness never bulk-deletes a shared
project. Use a dedicated test project and expire/delete it through its provider
after reviewing the report.

Reports under `reports/` contain the profile name, non-secret config
fingerprint, dataset identity, model names, per-question predictions and
recalled facts, failures, aggregate EM/F1/evidence metrics, and stage latency.
They never contain credentials and are not committed.

Live runs need network access, consume model/provider quota, and may take
minutes even for the smoke subset. A timeout or provider error remains a failed
run; it is not converted into a skip or pass.

## Offline validation

```sh
go test -race -tags gizclaw_locomo_e2e \
  -run 'TestDataset|TestScore|TestPreflight|TestRedaction|TestSession|TestRunBenchmark|TestAwait' \
  ./tests/locomo-e2e
bash -n tests/locomo-e2e/run_tests.sh
git lfs fsck
```
