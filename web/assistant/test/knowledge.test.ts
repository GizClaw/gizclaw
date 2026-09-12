import assert from "node:assert/strict";
import { test } from "node:test";

import { GUIDES_SITE, guideDocuments } from "../src/guides.ts";
import {
  chunkDocument,
  createKnowledgeIndex,
  tokenize,
} from "../src/knowledge.ts";
import { guidesIndex, readGuides } from "../testing/guides.ts";

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

test("guide documents link to the published site and skip unpublished pages", () => {
  const documents = guideDocuments({
    "zh/developing/monitor.md": "# Monitor\n正文",
    "zh/using/index.md": "---\nlayout: doc\n---\n# 使用\n正文",
    "zh/reviewing/examples/issue-example.md": "# 示例",
    "zh/notes.txt": "不是 Markdown",
  });
  assert.deepEqual(
    documents.map(({ id, title, source, url, text }) => ({
      id,
      title,
      source,
      url,
      text,
    })),
    [
      {
        id: "zh/developing/monitor.md",
        title: "Monitor",
        source: "guides/zh/developing/monitor.md",
        url: `${GUIDES_SITE}zh/developing/monitor`,
        text: "# Monitor\n正文",
      },
      {
        id: "zh/using/index.md",
        title: "使用",
        source: "guides/zh/using/index.md",
        url: `${GUIDES_SITE}zh/using/`,
        text: "# 使用\n正文",
      },
    ],
  );
});

test("the guides answer the questions the assistant meets", () => {
  assert.ok(Object.keys(readGuides()).length > 50);
  const sources = (query: string) =>
    guidesIndex()
      .search(query, 5)
      .map((passage) => passage.source);
  assert.ok(
    sources("DEBUG_ACCESS_FORBIDDEN 调试模式").includes(
      "guides/zh/developing/monitor.md",
    ),
  );
  assert.ok(
    sources("DEVICE_TIMEOUT 504").some((source) =>
      /api-keys|public/.test(source),
    ),
  );
  assert.ok(
    sources("Monitor Token gizclaw_mk_").includes(
      "guides/zh/developing/monitor.md",
    ),
  );
  const [passage] = guidesIndex().search("DEVICE_TIMEOUT", 1);
  assert.match(
    passage.url ?? "",
    /^https:\/\/gizclaw\.github\.io\/gizclaw\/zh\//,
  );
  assert.deepEqual(createKnowledgeIndex([]).search("任何问题"), []);
  assert.deepEqual(guidesIndex().search("   "), []);
});
