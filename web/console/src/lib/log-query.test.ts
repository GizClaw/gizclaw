import { describe, expect, it } from "vitest";
import {
  matches,
  parseQuery,
  summarize,
  type LogRecord,
} from "@/lib/log-query";
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

describe("log summary", () => {
  it("omits redundant successful RPC status", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "server.app_config.get",
          rpc_code: "0",
          result: "success",
          duration_ms: "2",
        },
      }),
    ).toBe("server.app_config.get · 2 ms");
  });

  it("retains RPC cancellation even with a successful code", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "server.app_config.get",
          rpc_code: "0",
          result: "canceled",
        },
      }),
    ).toBe("server.app_config.get · canceled");
  });

  it("explains application config not-found warnings", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "server.app_config.get",
          rpc_code: "5",
          duration_ms: "2",
          result: "client_error",
        },
      }),
    ).toBe("server.app_config.get · 未找到（RPC 5） · client_error · 2 ms");
  });

  it("keeps unknown RPC codes and backend error codes", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "future.operation",
          rpc_code: "99",
          error_code: "CUSTOM_ERROR",
        },
      }),
    ).toBe("future.operation · RPC 99 · CUSTOM_ERROR");
  });

  it("explains HTTP cancellation even with a successful status", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "getServerInfo",
          status: "200",
          result: "canceled",
        },
      }),
    ).toBe("getServerInfo · HTTP 200 · canceled");
  });

  it("preserves unstructured messages", () => {
    expect(
      summarize({ ...record, message: "connection lost", fields: undefined }),
    ).toBe("connection lost");
  });
});

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

describe("access log summaries", () => {
  it("shows the actual unmatched path and IP instead of unknown twice", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "unknown",
          method: "GET",
          route: "unknown",
          request_path: "/.env",
          client_ip: "192.0.2.10",
          status: "404",
          duration_ms: "0",
        },
      }),
    ).toBe("GET /.env · 192.0.2.10 · HTTP 404 · 0 ms");
  });
});

describe("uniform HTTP access format", () => {
  it.each(["200", "404"])(
    "uses the actual path for registered routes with status %s",
    (status) => {
      expect(
        summarize({
          ...record,
          fields: {
            method: "GET",
            operation: "getDevice",
            route: "/devices/:id",
            request_path: "/devices/device-123",
            client_ip: "192.0.2.10",
            status,
            duration_ms: "12",
          },
        }),
      ).toBe(`GET /devices/device-123 · 192.0.2.10 · HTTP ${status} · 12 ms`);
    },
  );
  it("retains the route fallback for historical HTTP records", () => {
    expect(
      summarize({
        ...record,
        fields: {
          method: "GET",
          operation: "getServerInfo",
          route: "/server-info",
          status: "200",
          duration_ms: "2",
        },
      }),
    ).toBe("GET /server-info · HTTP 200 · 2 ms");
  });
  it("retains operation names for RPC records", () => {
    expect(
      summarize({
        ...record,
        fields: {
          operation: "ping",
          rpc_code: "0",
          duration_ms: "1",
        },
      }),
    ).toBe("ping · 1 ms");
  });
});
