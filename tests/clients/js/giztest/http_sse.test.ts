import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { decodeHTTPEventStream } from "./http_sse.ts";

test("finite SSE projections match shared Go/Flutter fixtures", () => {
  const vectors: unknown = JSON.parse(
    readFileSync(
      new URL(
        "../../../../api/giztest/testdata/http_sse_vectors.json",
        import.meta.url,
      ),
      "utf8",
    ),
  );
  assert.ok(Array.isArray(vectors));
  for (const vector of vectors) {
    assert.ok(
      typeof vector === "object" &&
        vector !== null &&
        "text" in vector &&
        "expected" in vector,
    );
    assert.equal(typeof vector.text, "string");
    if (typeof vector.text !== "string") throw new Error("invalid fixture");
    assert.deepEqual(decodeHTTPEventStream(vector.text), vector.expected);
  }
});

test("finite SSE parsing rejects oversized responses and event counts", () => {
  assert.throws(
    () => decodeHTTPEventStream("x".repeat((4 << 20) + 1)),
    /exceeds 4 MiB/u,
  );
  assert.throws(
    () => decodeHTTPEventStream("data:x\n\n".repeat(16385)),
    /exceeds 16384 events/u,
  );
});
