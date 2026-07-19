# LoCoMo dataset

Use the upstream SNAP Research LoCoMo dataset from
<https://github.com/snap-research/locomo> and record the exact upstream commit
and SHA-256 checksum used for a run. The full dataset is not committed here.

This harness consumes Flowcraft eval JSONL, not the upstream JSON shape. Convert
the downloaded file with the pinned evaluator version:

```sh
go run github.com/GizClaw/flowcraft/eval/cmd/eval@v0.0.0-20260716084055-d6270ff568ec \
  locomo convert path/to/locomo10.json tests/locomo-e2e/testdata/locomo10.jsonl
```

The upstream repository currently declares the dataset under CC BY-NC 4.0;
verify the license and dataset identity again before sharing any derived data.
