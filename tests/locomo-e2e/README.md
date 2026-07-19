# Manual LoCoMo memory evaluation

This directory runs the pinned Flowcraft LoCoMo evaluator against GizClaw's
production `memory.Store` adapters. It is intentionally excluded from normal
`go test ./...`, `tests/gizclaw-e2e/run_tests.sh`, and required CI.

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

## Run

```sh
cp tests/locomo-e2e/.env.example tests/locomo-e2e/.env
# Fill one selected profile, dataset, answer model, and optional judge model.
bash tests/locomo-e2e/run_tests.sh
```

`run_tests.sh` only loads dotenv, performs common preflight, and invokes the
focused tagged Go test. No Python package or official LoCoMo Go library is
required. Dataset conversion and evaluation come from the pinned Flowcraft Go
module.

Each run uses unique opaque conversation scopes and a local sandbox. Remote
cleanup is deliberately conservative: the harness never bulk-deletes a shared
project. Use a dedicated test project and expire/delete it through its provider
after reviewing the report.

Reports under `reports/` contain the full profile name, non-secret config
fingerprint, dataset identity, timestamps, per-question failures, aggregate
EM/F1/judge metrics, and stage latency. They never contain credentials and are
not committed.

Live runs need network access, consume model/provider quota, and may take from
minutes (synthetic/subset) to hours (full LoCoMo). A timeout or provider error
remains a failed run; it is not converted into a skip or pass.

## Offline validation

```sh
go test -tags gizclaw_locomo_e2e \
  -run 'TestDataset|TestScore|TestPreflight|TestRedaction' \
  ./tests/locomo-e2e
bash -n tests/locomo-e2e/run_tests.sh
```
