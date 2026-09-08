import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LogView } from "@/components/log-view";

afterEach(cleanup);

it("shows and filters the meaning of structured warnings in the overview log", () => {
  render(
    <LogView
      entries={[
        {
          id: 1,
          time: "2026-09-08T11:26:43.139Z",
          level: "WARN",
          source: "上海 Server",
          message: "gizclaw: request completed",
          fields: {
            operation: "server.app_config.get",
            rpc_code: "5",
            duration_ms: "2",
          },
        },
      ]}
    />,
  );
  const summary = "server.app_config.get · 未找到（RPC 5） · 2 ms";
  expect(screen.getByText(summary)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("筛选日志"), {
    target: { value: "未找到" },
  });
  expect(screen.getByText(summary)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("筛选日志"), {
    target: { value: "request completed" },
  });
  expect(screen.getByText(summary)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("筛选日志"), {
    target: { value: "unrelated" },
  });
  expect(screen.queryByText(summary)).toBeNull();
});
