import { ScriptedModel } from "@gizclaw/assistant/testing";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { ConsoleAssistant } from "@/lib/config";

import AssistantPanel, { type AssistantPanelProps } from "./assistant-panel";
import { createAssistantStore, memoryRecords } from "./assistant-store";
import type { ConsoleStateDeps } from "./console-runtime";

const ASSISTANT: ConsoleAssistant = {
  apiKey: `gizclaw_sk_v1_${"a".repeat(40)}`,
  model: "llm",
};

const deps: ConsoleStateDeps = {
  config: () => ({
    name: "test",
    peers: [],
    servers: [
      {
        id: "edge-bj",
        name: "北京 Edge",
        url: "https://bj",
        role: "edge",
        monitorToken: "t",
      },
    ],
  }),
  fleet: () => ({
    "edge-bj": {
      status: "online",
      samples: [],
      snapshot: {
        public_key: "k",
        role: "edge",
        time: "",
        uptime_seconds: 1,
        goroutines: 1,
        heap_bytes: 1,
        transport: {
          connections: 512,
          services: 3,
          inbound_service_channels: 2,
          rx_bytes: 1,
          tx_bytes: 1,
        },
      },
    },
  }),
  peers: () => [],
  peerStates: () => ({}),
  watch: vi.fn(),
  view: { current: { page: "集群总览" } },
  window,
  now: Date.now,
};

// jsdom lacks ResizeObserver, which assistant-ui uses to keep the thread
// scrolled to the latest message.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;
// Nor does it scroll, which a restored conversation triggers.
Element.prototype.scrollTo ??= () => {};

beforeEach(() => {
  window.location.hash = "";
});
afterEach(cleanup);

function Panel(props: Partial<AssistantPanelProps>) {
  return (
    <AssistantPanel
      assistant={ASSISTANT}
      runtimeDeps={deps}
      endpoint="https://node.example.com"
      onClose={() => {}}
      onOpenConfig={() => {}}
      store={createAssistantStore(memoryRecords())}
      {...props}
    />
  );
}

async function ask(text: string) {
  const input = await screen.findByLabelText("向诊断助手提问");
  fireEvent.change(input, { target: { value: text } });
  fireEvent.click(screen.getByLabelText("发送"));
}

describe("assistant panel", () => {
  it("runs a turn that reads the console, navigates the page and shows its actions", async () => {
    const model = new ScriptedModel([
      { call: [{ name: "list_nodes", arguments: {} }] },
      {
        say: "北京 Edge 最忙：512 个连接。",
        call: [
          {
            name: "navigate",
            arguments: { page: "server", node_id: "edge-bj" },
          },
        ],
      },
      { reply: "已打开它的节点详情页。" },
    ]);
    const createModel = vi.fn(() => model);
    render(
      <Panel endpoint="https://node.example.com/" createModel={createModel} />,
    );
    await ask("哪个节点最忙？");

    await screen.findByText(/已打开它的节点详情页/);
    expect(window.location.hash).toBe("#/server/edge-bj");
    expect(screen.getByText(/512 个连接/)).toBeTruthy();
    expect(screen.getByText("读取节点状态")).toBeTruthy();
    expect(screen.getByText(/跳转页面 → #\/server\/edge-bj/)).toBeTruthy();
    // The model received the node list the console holds.
    const toolResult = JSON.stringify(model.requests[1].input);
    expect(toolResult).toContain("512");
    expect(createModel).toHaveBeenCalledTimes(1);
    expect(createModel).toHaveBeenCalledWith(
      ASSISTANT,
      "https://node.example.com/openai/v1",
    );
  });

  it("saves conversations, starts new ones and switches between them", async () => {
    // Every session gets its own model, answering with its sequence number.
    const models: InstanceType<typeof ScriptedModel>[] = [];
    const store = createAssistantStore(memoryRecords());
    const panel = (
      <Panel
        store={store}
        createModel={() => {
          const model = new ScriptedModel([
            { reply: `会话 ${models.length + 1} 的回答` },
          ]);
          models.push(model);
          return model;
        }}
      />
    );
    const view = render(panel);
    await ask("客厅音箱怎么了");
    await screen.findByText("会话 1 的回答");
    await waitFor(async () =>
      expect(await store.listThreads()).toHaveLength(1),
    );

    fireEvent.click(screen.getByLabelText("新对话"));
    await waitFor(() => expect(screen.queryByText("会话 1 的回答")).toBeNull());
    await ask("另一个问题");
    await screen.findByText("会话 2 的回答");
    // The new conversation's first request carries only its own message.
    expect(models[1].requests[0].input).toHaveLength(1);
    await waitFor(async () =>
      expect(await store.listThreads()).toHaveLength(2),
    );

    // Reopening the panel restores the latest conversation from the store.
    view.unmount();
    render(panel);
    await screen.findByText("会话 2 的回答");

    fireEvent.click(screen.getByLabelText("历史对话"));
    fireEvent.click(await screen.findByText("客厅音箱怎么了"));
    await screen.findByText("会话 1 的回答");
    await ask("继续");
    const last = models.length;
    await screen.findByText(`会话 ${last} 的回答`);
    // The restored session resumes the saved context.
    const resumed = JSON.stringify(models[last - 1].requests[0].input);
    expect(resumed).toContain("客厅音箱怎么了");
    expect(resumed).toContain("会话 1 的回答");
  });

  it("deletes one conversation or clears them all", async () => {
    const store = createAssistantStore(memoryRecords());
    const now = Date.now();
    for (const [index, title] of ["旧对话", "新一点的对话"].entries()) {
      await store.saveThread({
        id: `t${index}`,
        title,
        createdAt: now + index,
        updatedAt: now + index,
        entries: [{ id: `e${index}`, role: "user", text: title }],
        history: [],
      });
    }
    render(<Panel store={store} createModel={() => new ScriptedModel([])} />);
    await screen.findByText("新一点的对话");
    fireEvent.click(screen.getByLabelText("历史对话"));
    fireEvent.click(await screen.findByLabelText("删除对话 旧对话"));
    await waitFor(() => expect(screen.queryByText("旧对话")).toBeNull());
    expect((await store.listThreads()).map((item) => item.id)).toEqual(["t1"]);

    fireEvent.click(screen.getByText("清空全部对话"));
    fireEvent.click(screen.getByText("确认清空全部对话"));
    await waitFor(async () => expect(await store.listThreads()).toEqual([]));
    expect(await store.loadThread("t1")).toBeUndefined();
    expect(screen.queryByText("新一点的对话")).toBeNull();
  });

  it("imports knowledge the assistant can search", async () => {
    const store = createAssistantStore(memoryRecords());
    const model = new ScriptedModel([
      {
        call: [
          { name: "search_knowledge", arguments: { query: "阳台音箱 信号弱" } },
        ],
      },
      { reply: "按手册切换到 2.4G。" },
    ]);
    render(<Panel store={store} createModel={() => model} />);
    fireEvent.click(await screen.findByLabelText("知识库"));
    expect(screen.getByText("设备控制错误码")).toBeTruthy();
    const file = new File(
      ["# 阳台音箱排障\n\n## 信号弱\n阳台音箱信号弱时切换到 2.4G 网络。"],
      "runbook.md",
      { type: "text/markdown" },
    );
    fireEvent.change(screen.getByLabelText("选择知识库文档"), {
      target: { files: [file] },
    });
    await screen.findByText("阳台音箱排障");
    expect(await store.listKnowledge()).toMatchObject([
      { title: "阳台音箱排障", source: "runbook.md" },
    ]);

    fireEvent.click(screen.getByLabelText("知识库"));
    await ask("阳台音箱信号弱怎么办");
    await screen.findByText("按手册切换到 2.4G。");
    expect(screen.getByText("检索知识库")).toBeTruthy();
    expect(JSON.stringify(model.requests[1].input)).toContain("runbook.md");

    fireEvent.click(screen.getByLabelText("知识库"));
    fireEvent.click(screen.getByLabelText("删除文档 阳台音箱排障"));
    await waitFor(async () => expect(await store.listKnowledge()).toEqual([]));
  });

  it("explains how to configure the assistant when the config has none", async () => {
    const onOpenConfig = vi.fn();
    render(
      <AssistantPanel
        runtimeDeps={deps}
        endpoint="https://node.example.com"
        onClose={() => {}}
        onOpenConfig={onOpenConfig}
      />,
    );
    expect(screen.getByText(/已有设备的 API Key/)).toBeTruthy();
    expect(screen.queryByLabelText("向诊断助手提问")).toBeNull();
    fireEvent.click(screen.getByText("编辑配置"));
    expect(onOpenConfig).toHaveBeenCalled();
  });

  it("shows a failed turn with the actions it took", async () => {
    render(
      <Panel
        createModel={() =>
          new ScriptedModel([{ call: [{ name: "list_nodes", arguments: {} }] }])
        }
      />,
    );
    await ask("看看节点");
    await waitFor(() => expect(screen.getByText(/助手出错/)).toBeTruthy());
    expect(screen.getByText("读取节点状态")).toBeTruthy();
  });

  it("keeps the conversation when the assistant configuration changes", async () => {
    const store = createAssistantStore(memoryRecords());
    const models: InstanceType<typeof ScriptedModel>[] = [];
    const createModel = () => {
      const model = new ScriptedModel([
        { reply: `模型 ${models.length + 1} 的回答` },
      ]);
      models.push(model);
      return model;
    };
    const view = render(<Panel store={store} createModel={createModel} />);
    await ask("第一个问题");
    await screen.findByText("模型 1 的回答");

    view.rerender(
      <Panel
        store={store}
        createModel={createModel}
        assistant={{ ...ASSISTANT, model: "chat" }}
      />,
    );
    await screen.findByText("模型 1 的回答");
    await ask("第二个问题");
    await screen.findByText("模型 2 的回答");
    expect(JSON.stringify(models[1].requests[0].input)).toContain(
      "模型 1 的回答",
    );
  });
});
