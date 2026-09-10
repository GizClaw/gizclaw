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
        workflow_id: "flow",
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

const requests = () =>
  loadHistory.mock.calls.map((call) => call[3] as HistoryRequest);

afterEach(cleanup);

describe("peer chat timeline", () => {
  beforeEach(() => {
    loadHistory.mockReset();
  });

  it("asks the server for newest entries first", async () => {
    loadHistory.mockResolvedValue({
      available: true,
      items: [entry("b", "second"), entry("a", "first")],
      has_next: false,
    });
    render(<PeerChat peer={peer} />);
    await screen.findByText("second");
    expect(requests()).toEqual([{ query: "", order: "desc" }]);
    const texts = screen
      .getAllByText(/first|second/)
      .map((node) => node.textContent);
    expect(texts).toEqual(["second", "first"]);
    expect(screen.queryByText("更新的记录")).toBeNull();
  });

  it("jumps to a day and pages older and newer on the server", async () => {
    loadHistory.mockImplementation(
      (_endpoint: string, _key: string, _ws: string, req: HistoryRequest) => {
        if (req.endTimeMs !== undefined) {
          return Promise.resolve({
            available: true,
            items: [entry("d2", "day two")],
            has_next: true,
            next_cursor: "d2",
          });
        }
        if (req.order === "asc" && req.limit === 1) {
          return Promise.resolve({
            available: true,
            items: [entry("d3", "day three")],
            has_next: true,
          });
        }
        if (req.order === "asc") {
          return Promise.resolve({
            available: true,
            items: [entry("d3", "day three"), entry("d4", "day four")],
            has_next: false,
          });
        }
        if (req.cursor === "d2") {
          return Promise.resolve({
            available: true,
            items: [entry("d1", "day one")],
            has_next: false,
          });
        }
        return Promise.resolve({ available: true, items: [], has_next: false });
      },
    );
    render(<PeerChat peer={peer} />);
    await waitFor(() => expect(loadHistory).toHaveBeenCalledTimes(1));

    fireEvent.change(screen.getByLabelText("跳到日期"), {
      target: { value: "2026-06-02" },
    });
    await screen.findByText("day two");
    const end = new Date(2026, 5, 3).getTime();
    expect(requests().slice(1)).toEqual([
      { query: "", order: "desc", endTimeMs: end },
      { query: "", order: "asc", startTimeMs: end, limit: 1 },
    ]);

    fireEvent.click(screen.getByText("更早的记录"));
    await screen.findByText("day one");
    expect(requests().at(-1)).toEqual({
      query: "",
      order: "desc",
      cursor: "d2",
    });

    fireEvent.click(screen.getByText("更新的记录"));
    await screen.findByText("day four");
    expect(requests().at(-1)).toEqual({
      query: "",
      order: "asc",
      cursor: "d2",
      startTimeMs: undefined,
    });
    const texts = screen.getAllByText(/^day /).map((node) => node.textContent);
    expect(texts).toEqual(["day four", "day three", "day two", "day one"]);
    expect(screen.queryByText("更新的记录")).toBeNull();
    expect(screen.queryByText("更早的记录")).toBeNull();
  });
});
