"""Assert the recovery evidence remains visible with system_log.level=warn."""
import pathlib
import shlex
import sys

text = pathlib.Path(sys.argv[1]).read_text()
records = []
for line in text.splitlines():
    if "doubao asr:" not in line:
        continue
    fields = dict(part.split("=", 1) for part in shlex.split(line) if "=" in part)
    records.append(fields)
assert [r["msg"] for r in records] == [
    "doubao asr: attempt failed", "doubao asr: retry start", "doubao asr: retry success"
], records
assert [r["retry"] for r in records] == ["0", "1", "1"], records
for key in ("asr_request_id", "session_id", "workspace", "input_stream_id", "transcript_stream_id", "segment_index"):
    assert records[0].get(key), (key, records)
    assert len({r.get(key) for r in records}) == 1, (key, records)
assert all(r["level"] == "WARN" for r in records), records
assert records[0]["error_code"] == "55000000", records
assert records[0]["upstream_reqid"] == records[0]["upstream_connect_id"], records
assert records[0]["upstream_reqid"] != records[0]["asr_request_id"], records
assert records[2]["upstream_reqid"] != records[0]["upstream_reqid"], records
assert not records[0]["upstream_trace_id"] and not records[0]["upstream_log_id"], records
assert "asr recovery fixture: replay verified" in text, text
assert "assistant route failed" not in text and "output consumer failed" not in text, text
print("ASR recovery WARN logs: original failure, retry=1 start/success and exact replay verified")
