import { describe, expect, it } from "vitest";

import {
  createAssistantStore,
  knowledgeDocument,
  MAX_THREADS,
  memoryRecords,
  threadTitle,
  type StoredThread,
} from "./assistant-store";

const thread = (index: number): StoredThread => ({
  id: `t${index}`,
  title: `对话 ${index}`,
  createdAt: index,
  updatedAt: index,
  entries: [],
  history: [],
});

describe("assistant store", () => {
  it("keeps the most recent threads and drops the oldest", async () => {
    const records = memoryRecords();
    const store = createAssistantStore(records);
    for (let index = 0; index <= MAX_THREADS; index++) {
      await store.saveThread(thread(index));
    }
    const list = await store.listThreads();
    expect(list).toHaveLength(MAX_THREADS);
    expect(list[0].id).toBe(`t${MAX_THREADS}`);
    expect(await store.loadThread("t0")).toBeUndefined();
    expect(await store.loadThread("t1")).toMatchObject({ title: "对话 1" });
  });

  it("serializes concurrent saves so none is lost", async () => {
    const store = createAssistantStore(memoryRecords());
    await Promise.all(
      [1, 2, 3].map((index) => store.saveThread(thread(index))),
    );
    expect((await store.listThreads()).map((item) => item.id)).toEqual([
      "t3",
      "t2",
      "t1",
    ]);
  });

  it("clears threads but keeps imported knowledge", async () => {
    const store = createAssistantStore(memoryRecords());
    await store.saveThread(thread(1));
    const document = knowledgeDocument("手册.md", "# 排障手册\n内容", "x");
    await store.saveKnowledge([document]);
    await store.clearThreads();
    expect(await store.listThreads()).toEqual([]);
    expect(await store.loadThread("t1")).toBeUndefined();
    expect(await store.listKnowledge()).toEqual([document]);
  });

  it("titles threads and documents", () => {
    expect(threadTitle([])).toBe("新对话");
    expect(
      threadTitle([{ id: "1", role: "user", text: `  ${"很".repeat(40)}\n` }]),
    ).toBe(`${"很".repeat(30)}…`);
    expect(knowledgeDocument("手册.md", "# 排障手册\n内容", "x")).toEqual({
      id: "import/x",
      title: "排障手册",
      source: "手册.md",
      text: "# 排障手册\n内容",
    });
    expect(knowledgeDocument("notes.txt", "没有标题", "y").title).toBe("notes");
  });
});
