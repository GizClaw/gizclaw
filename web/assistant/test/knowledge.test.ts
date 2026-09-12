import assert from "node:assert/strict";
import { test } from "node:test";

import { BUILTIN_KNOWLEDGE } from "../src/builtin-knowledge.ts";
import {
  chunkDocument,
  createKnowledgeIndex,
  tokenize,
} from "../src/knowledge.ts";

test("tokenize pairs CJK characters and splits compound identifiers", () => {
  assert.deepEqual(tokenize("调试模式"), ["调试", "试模", "模式"]);
  assert.deepEqual(tokenize("DEBUG_ACCESS_FORBIDDEN"), [
    "debug_access_forbidden",
    "debug",
    "access",
    "forbidden",
  ]);
  assert.deepEqual(tokenize("network.rssi_dbm 弱"), [
    "network.rssi_dbm",
    "network",
    "rssi",
    "dbm",
    "弱",
  ]);
});

test("chunkDocument keeps the heading path of every section", () => {
  const passages = chunkDocument({
    id: "d",
    title: "文档",
    source: "测试",
    text: "# 一级\n开头\n## 二级\n内容 A\n\n### 三级\n内容 B\n## 另一个二级\n内容 C",
  });
  assert.deepEqual(
    passages.map((passage) => [passage.heading, passage.text]),
    [
      ["一级", "开头"],
      ["一级 / 二级", "内容 A"],
      ["一级 / 二级 / 三级", "内容 B"],
      ["一级 / 另一个二级", "内容 C"],
    ],
  );
});

test("chunkDocument splits long sections", () => {
  const long = Array.from(
    { length: 20 },
    (_, index) => `段落${index} ${"字".repeat(80)}`,
  ).join("\n\n");
  const passages = chunkDocument({
    id: "d",
    title: "t",
    source: "s",
    text: long,
  });
  assert.ok(passages.length > 1);
  assert.ok(passages.every((passage) => passage.text.length <= 700));
});

test("the built-in knowledge answers the questions the assistant meets", () => {
  const index = createKnowledgeIndex(BUILTIN_KNOWLEDGE);
  const top = (query: string) => index.search(query, 1)[0];
  assert.equal(
    top("DEBUG_ACCESS_FORBIDDEN 是什么意思")?.title,
    "设备调试模式与访问权限",
  );
  assert.equal(top("设备返回 DEVICE_TIMEOUT")?.title, "设备控制错误码");
  assert.equal(
    top("节点状态 503 MONITOR_DISABLED")?.title,
    "节点监控与 Monitor Token",
  );
  assert.equal(top("network.rssi_dbm 这个字段")?.title, "Telemetry 与设备日志");
  assert.deepEqual(index.search("   "), []);
});

test("imported documents are searched with the built-in ones", () => {
  const index = createKnowledgeIndex([
    ...BUILTIN_KNOWLEDGE,
    {
      id: "import/runbook",
      title: "阳台音箱排障手册",
      source: "runbook.md",
      text: "# 信号弱\n阳台音箱在路由器 5G 覆盖边缘，先切换到 2.4G 网络。",
    },
  ]);
  const [first] = index.search("阳台音箱 信号弱怎么办");
  assert.equal(first.source, "runbook.md");
  assert.equal(first.heading, "信号弱");
  assert.ok(first.score > 0);
});
