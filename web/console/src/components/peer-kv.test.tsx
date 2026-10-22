import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { PeerKV } from "./peer-kv";
afterEach(cleanup);
it("distinguishes absent data and reported null, false, zero and nested JSON", () => {
  const { rerender } = render(<PeerKV value={undefined} />);
  expect(screen.getByText("暂无自报数据")).toBeTruthy();
  const value = Object.fromEntries(
    [null, false, 0, { a: [null, false, 0] }].map((value, index) => [
      `key-${index}`,
      { value, observed_at: "2026-10-22T00:00:00Z" },
    ]),
  );
  rerender(<PeerKV value={value} />);
  expect(screen.queryByText("暂无自报数据")).toBeNull();
  expect(screen.getByText("key-0")).toBeTruthy();
  expect(screen.getAllByText("null").length).toBeGreaterThan(0);
  expect(screen.getAllByText("false").length).toBeGreaterThan(0);
  expect(screen.getAllByText("0").length).toBeGreaterThan(0);
  expect(screen.getByText(/"a":/)).toBeTruthy();
  expect(document.querySelectorAll("details").length).toBe(4);
});

it("renders lossless raw JSON numbers", () => {
  render(
    <PeerKV
      value={{
        huge: {
          value: { number: Infinity },
          json_value: '{"number":1e400}',
          observed_at: "2026-10-22T00:00:00Z",
        },
      }}
    />,
  );
  expect(screen.getByText('{"number":1e400}')).toBeTruthy();
});
