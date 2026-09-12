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

import AssistantPanel from "./assistant-panel";
import type { ConsoleRuntimeDeps } from "./console-runtime";

const ASSISTANT: ConsoleAssistant = {
  apiKey: `gizclaw_sk_v1_${"a".repeat(40)}`,
  model: "llm",
};

const deps: Omit<ConsoleRuntimeDeps, "signal"> = {
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

beforeEach(() => {
  window.location.hash = "";
});
afterEach(cleanup);

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
      <AssistantPanel
        assistant={ASSISTANT}
        runtimeDeps={deps}
        endpoint="https://node.example.com/"
        onClose={() => {}}
        onOpenConfig={() => {}}
        createModel={createModel}
      />,
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
      <AssistantPanel
        assistant={ASSISTANT}
        runtimeDeps={deps}
        endpoint="https://node.example.com"
        onClose={() => {}}
        onOpenConfig={() => {}}
        createModel={() =>
          new ScriptedModel([{ call: [{ name: "list_nodes", arguments: {} }] }])
        }
      />,
    );
    await ask("看看节点");
    await waitFor(() => expect(screen.getByText(/助手出错/)).toBeTruthy());
    expect(screen.getByText("读取节点状态")).toBeTruthy();
  });
});
