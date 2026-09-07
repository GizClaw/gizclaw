import { describe, expect, it } from "vitest";
import { matches, parseQuery, type LogRecord } from "@/lib/log-query";
import { parseWatchedPeers } from "@/lib/peers";

const record: LogRecord = {
  id: 12,
  time: "2026-09-07T18:10:32.968Z",
  level: "INFO",
  message: "gizclaw: request completed",
  peer_public_key: "2WheeWYEzpnZMaNi",
  fields: {
    request_id: "28952ddc7bf5f516",
    operation: "getServerInfo",
    route: "/server-info",
    status: "200",
    surface: "edge-http",
  },
  node: "edge-1",
  nodeName: "Edge 节点",
};

describe("log filter", () => {
  it("matches free text across message, error, peer and fields", () => {
    expect(matches(record, parseQuery("getserverinfo"))).toBe(true);
    expect(matches(record, parseQuery("edge-http"))).toBe(true);
    expect(matches(record, parseQuery("nothing here"))).toBe(false);
  });

  it("matches field clauses and excludes with a leading dash", () => {
    expect(matches(record, parseQuery("status:200"))).toBe(true);
    expect(matches(record, parseQuery("status:500"))).toBe(false);
    expect(matches(record, parseQuery("-status:200"))).toBe(false);
    expect(matches(record, parseQuery("-status:500"))).toBe(true);
    expect(matches(record, parseQuery("request_id:28952ddc7bf5f516"))).toBe(
      true,
    );
  });

  it("resolves record columns as well as structured fields", () => {
    expect(matches(record, parseQuery("node:edge-1 level:INFO"))).toBe(true);
    expect(matches(record, parseQuery("peer:2WheeWYEzpnZMaNi"))).toBe(true);
    expect(matches(record, parseQuery("node:server-1"))).toBe(false);
  });

  it("requires every term, and treats an unparsable token as text", () => {
    expect(matches(record, parseQuery("status:200 getServerInfo"))).toBe(true);
    expect(matches(record, parseQuery("status:200 missing"))).toBe(false);
    expect(parseQuery("status:").text).toEqual(["status:"]);
    expect(parseQuery("  ").clauses).toEqual([]);
  });
});

describe("watched peers", () => {
  it("reads a stored list and ignores malformed content", () => {
    const stored = JSON.stringify([
      {
        publicKey: "abc",
        label: "工位机",
        endpoint: "https://ap.gizclaw.com",
        addedAt: 1,
      },
    ]);
    expect(parseWatchedPeers(stored)).toEqual([
      {
        publicKey: "abc",
        label: "工位机",
        endpoint: "https://ap.gizclaw.com",
        addedAt: 1,
      },
    ]);
    expect(parseWatchedPeers("")).toEqual([]);
    expect(parseWatchedPeers(JSON.stringify([{ label: "no key" }]))).toEqual(
      [],
    );
    expect(parseWatchedPeers("not json")).toEqual([]);
  });
});
