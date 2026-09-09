import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LogStream } from "@/components/log-stream";
import { matches, parseQuery, type LogRecord } from "@/lib/log-query";

afterEach(cleanup);

it("shows and filters the meaning of structured warnings in the overview log", () => {
  const record: LogRecord = {
    id: 1,
    time: "2026-09-08T11:26:43.139Z",
    level: "WARN",
    node: "server",
    nodeName: "上海 Server",
    message: "gizclaw: request completed",
    fields: {
      operation: "server.app_config.get",
      rpc_code: "5",
      duration_ms: "2",
    },
  };
  render(
    <LogStream records={[record]} onSelect={() => {}} emptyMessage="empty" />,
  );
  expect(
    screen.getByText("server.app_config.get · 未找到（RPC 5） · 2 ms"),
  ).toBeTruthy();
  expect(matches(record, parseQuery("未找到"))).toBe(true);
  expect(matches(record, parseQuery("request completed"))).toBe(true);
  expect(matches(record, parseQuery("unrelated"))).toBe(false);
});
