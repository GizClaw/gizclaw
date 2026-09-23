import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";
import YAML from "yaml";

function record(value: unknown): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new Error("expected a YAML object");
  }
  return value as Record<string, unknown>;
}

const fixture = new URL(
  "../../testdata/resources/04-workflows/32-giztest-workflow-tester.yaml",
  import.meta.url,
);
const document: unknown = YAML.parse(readFileSync(fixture, "utf8"));
const nodes = record(record(record(record(document).spec).flowcraft).graph).nodes;
if (!Array.isArray(nodes)) {
  throw new Error("tester workflow has no graph nodes");
}
const publisher = nodes.find((node: unknown) => record(node).id === "publish_response");
const source = record(record(publisher).config).source;
if (typeof source !== "string") {
  throw new Error("tester workflow has no publisher script");
}

function publish(mode: string, response: string): string {
  const emitted: string[] = [];
  runInNewContext(
    source,
    {
      board: {
        getVar(name: string): string {
          return name === "tester_mode" ? mode : response;
        },
      },
      host: {
        emit(event: string, payload: unknown): void {
          assert.equal(event, "token");
          const content = record(payload).content;
          assert.equal(typeof content, "string");
          emitted.push(content as string);
        },
      },
    },
    { timeout: 1000 },
  );
  assert.equal(emitted.length, 1);
  return emitted[0];
}

test("tester probes cannot publish an early verdict or empty reply", () => {
  for (const response of ["PASS", "FAIL", "  pass  ", ""]) {
    assert.equal(publish("probe", response), "请就刚才的话题再补充一点好吗？");
  }
  assert.equal(publish("probe", "请再介绍一条线索。"), "请再介绍一条线索。");
});

test("tester verdict is published without rewriting", () => {
  assert.equal(publish("verdict", "PASS"), "PASS");
  assert.equal(publish("verdict", "FAIL"), "FAIL");
});
