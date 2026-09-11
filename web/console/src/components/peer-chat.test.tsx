import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { HistoryRequest } from "@/lib/peers";
import { PeerChat } from "@/components/peer-chat";

const loadHistory = vi.fn();
vi.mock("@/lib/peers", () => ({
  loadWorkspaces: () =>
    Promise.resolve([
      {
        id: "ws",
        name: "Pet",
        collection: "pets",
        workflow_name: "pet.cat",
        available: true,
        system: true,
        last_active_at: new Date(0).toISOString(),
      },
    ]),
  loadHistory: (...args: unknown[]) => loadHistory(...args),
  loadHistoryAudio: vi.fn(),
}));

const peer = {
  publicKey: "device-key",
  label: "Device",
  endpoint: "https://edge.example.com",
  addedAt: 1,
};

const entry = (name: string, text: string) => ({
  name,
  actor_name: "assistant",
  type: "agent",
  text,
  created_at: new Date(2026, 5, 2, 12).toISOString(),
  replay_available: false,
});

const page = (items: ReturnType<typeof entry>[], nextCursor?: string) => ({
  available: true,
  items,
  has_next: nextCursor !== undefined,
  next_cursor: nextCursor,
});

const requests = () =>
  loadHistory.mock.calls.map((call) => call[3] as HistoryRequest);

// Search hits split their text around <mark>, so match whole rows.
const hitRow = (text: string) =>
  screen.findByText(
    (_, element) => element?.tagName === "SPAN" && element.textContent === text,
  );

const texts = (pattern: RegExp) =>
  screen.getAllByText(pattern).map((node) => node.textContent);

afterEach(cleanup);

describe("peer chat", () => {
  beforeEach(() => {
    loadHistory.mockReset();
  });

  it("asks the server for newest entries first", async () => {
    loadHistory.mockResolvedValue(
      page([entry("b", "second"), entry("a", "first")]),
    );
    render(<PeerChat peer={peer} />);
    await screen.findByText("second");
    expect(requests()).toEqual([{ query: "", order: "desc" }]);
    expect(texts(/^(first|second)$/)).toEqual(["second", "first"]);
    expect(screen.queryByText("更新的记录")).toBeNull();
  });

  it("lists search hits and locates a hit inside its context", async () => {
    const hit = entry("m3", "needle three");
    loadHistory.mockImplementation(
      (_endpoint: string, _key: string, _ws: string, req: HistoryRequest) => {
        if (req.query === "needle") {
          return Promise.resolve(page([hit, entry("m1", "needle one")]));
        }
        if (req.cursor === "m3" && req.order === "asc") {
          return Promise.resolve(
            page([entry("m4", "line four"), entry("m5", "line five")], "m5"),
          );
        }
        if (req.cursor === "m3" && req.order === "desc") {
          return Promise.resolve(page([entry("m2", "line two")]));
        }
        if (req.cursor === "m5" && req.order === "asc") {
          return Promise.resolve(page([entry("m6", "line six")]));
        }
        return Promise.resolve(page([entry("m9", "line latest")]));
      },
    );
    render(<PeerChat peer={peer} />);
    await screen.findByText("line latest");

    fireEvent.change(screen.getByLabelText("搜索聊天记录"), {
      target: { value: " needle " },
    });
    fireEvent.click(screen.getByText("搜索"));
    await hitRow("needle one");
    expect(requests().at(-1)).toEqual({ query: "needle", order: "desc" });
    expect(screen.queryByText("line latest")).toBeNull();

    fireEvent.click(await hitRow("needle three"));
    await screen.findByText("line two");
    expect(requests().slice(-2)).toEqual([
      { query: "", order: "asc", cursor: "m3", limit: 20 },
      { query: "", order: "desc", cursor: "m3", limit: 40 },
    ]);
    expect(texts(/^(line \w+|needle three)$/)).toEqual([
      "line five",
      "line four",
      "needle three",
      "line two",
    ]);
    expect(
      screen.getByText("needle three").closest("[aria-current]"),
    ).not.toBeNull();

    fireEvent.click(screen.getByText("更新的记录"));
    await screen.findByText("line six");
    expect(requests().at(-1)).toEqual({
      query: "",
      order: "asc",
      cursor: "m5",
    });
    expect(screen.queryByText("更新的记录")).toBeNull();

    const before = loadHistory.mock.calls.length;
    fireEvent.click(screen.getByText("返回搜索结果"));
    await hitRow("needle one");
    expect(loadHistory.mock.calls.length).toBe(before);

    fireEvent.click(screen.getByText("清除搜索"));
    await waitFor(() => expect(screen.queryByText("返回搜索结果")).toBeNull());
    fireEvent.click(screen.getByText("回到最新"));
    await screen.findByText("line latest");
    expect(requests().at(-1)).toEqual({ query: "", order: "desc" });
  });
});
